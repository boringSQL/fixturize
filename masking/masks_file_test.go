package masking

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile is a small test helper that creates any missing parent
// directories and then writes the given content to the specified path.
// It uses t.Helper() so that any failure is reported at the call site
// in the test function rather than inside this helper, which makes the
// resulting test output much easier to read and debug.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestLoadSharedMasks_HappyPath verifies the canonical "everything is
// well-formed" scenario for LoadSharedMasks. It constructs a temporary
// data-masking-policy.yml file describing a single logical database
// named "app" that contains two columns with different tags (one tagged
// "pii" and one tagged "internal") and a single policy named "pii"
// which includes the "pii" tag. The test then loads the file and
// asserts that every field on the resulting SharedMasksFile struct
// matches what was written to disk, including the version number, the
// number of databases, the number of columns per database, the tag
// values on each column, and the contents of the policy's IncludeTags
// slice. This serves as the primary regression guard for the YAML
// schema mapping.
func TestLoadSharedMasks_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MasksFileName)
	writeFile(t, path, `
version: 1
databases:
  app:
    columns:
      users.email:
        expr: "'***'"
        tags: [pii]
      users.role:
        expr: "'internal'"
        tags: [internal]
    policies:
      pii:
        include_tags: [pii]
`)

	f, err := LoadSharedMasks(path)
	if err != nil {
		t.Fatalf("LoadSharedMasks: %v", err)
	}
	if f.Version != 1 {
		t.Fatalf("version = %d, want 1", f.Version)
	}
	if len(f.Databases) != 1 {
		t.Fatalf("databases = %d, want 1", len(f.Databases))
	}

	app, ok := f.Databases["app"]
	if !ok {
		t.Fatalf("missing 'app' database")
	}
	if len(app.Columns) != 2 {
		t.Fatalf("columns = %d, want 2", len(app.Columns))
	}

	email := app.Columns["users.email"]
	if len(email.Tags) != 1 || email.Tags[0] != "pii" {
		t.Fatalf("users.email tags = %v, want [pii]", email.Tags)
	}
	role := app.Columns["users.role"]
	if len(role.Tags) != 1 || role.Tags[0] != "internal" {
		t.Fatalf("users.role tags = %v, want [internal]", role.Tags)
	}

	pol, ok := app.Policies["pii"]
	if !ok {
		t.Fatalf("missing 'pii' policy")
	}
	if len(pol.IncludeTags) != 1 || pol.IncludeTags[0] != "pii" {
		t.Fatalf("pii policy include_tags = %v, want [pii]", pol.IncludeTags)
	}
}

// TestLoadSharedMasks_VersionMismatch ensures that LoadSharedMasks
// refuses to silently accept a policy file whose top-level "version"
// field is set to anything other than 1. The masking package
// intentionally treats the version number as a hard schema contract:
// future schema changes will bump the version, and old binaries should
// fail loudly rather than misinterpreting newer files. The test writes
// a syntactically valid YAML document with version: 2 and confirms
// that the returned error message contains the substring
// "unsupported masks file version 2", which is the contract that
// downstream tooling and human operators rely on.
func TestLoadSharedMasks_VersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MasksFileName)
	writeFile(t, path, "version: 2\ndatabases: {}\n")

	_, err := LoadSharedMasks(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported masks file version 2") {
		t.Fatalf("error = %q, want contains %q", err.Error(), "unsupported masks file version 2")
	}
}

// TestLoadSharedMasks_MissingFile validates the behavior of
// LoadSharedMasks when the requested file simply does not exist on
// disk. Callers in fixturize and in downstream consumers such as
// dryrun rely on being able to distinguish "file genuinely not
// present" from "file present but unreadable or malformed" so that
// they can transparently fall back to a default policy when no
// shared masks file has been configured. To keep that contract
// stable, the error returned by LoadSharedMasks must wrap the
// underlying filesystem error in a way that errors.Is(err,
// fs.ErrNotExist) returns true. The test exercises that contract by
// pointing LoadSharedMasks at a path inside a fresh temporary
// directory that is guaranteed not to contain the file.
func TestLoadSharedMasks_MissingFile(t *testing.T) {
	_, err := LoadSharedMasks(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want fs.ErrNotExist", err)
	}
}

// TestLoadSharedMasks_Malformed verifies that when the policy file
// exists but contains content that the YAML parser cannot make sense
// of, LoadSharedMasks returns a non-nil error whose message contains
// the substring "failed to parse masks file". This contract is
// important because the path to the offending file is wrapped into
// the error message, and operators rely on that path to quickly
// locate and fix the broken configuration in larger repositories
// where several policy files may exist at different levels.
func TestLoadSharedMasks_Malformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MasksFileName)
	writeFile(t, path, "::: not yaml :::\n\t- [unbalanced\n")

	_, err := LoadSharedMasks(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse masks file") {
		t.Fatalf("error = %q, want contains %q", err.Error(), "failed to parse masks file")
	}
}

// TestDiscoverMasksFile_InStartDir covers the simplest and most
// common discovery scenario: the policy file lives directly inside
// the directory that DiscoverMasksFile is asked to start the search
// from, so no upward walking is required at all. A .git directory
// is also planted inside the temporary directory; this serves as a
// safety cap that prevents the discovery walk from accidentally
// escaping into the host repository in the (extremely unlikely)
// event of a logic bug, which would make the test environment-
// dependent and therefore flaky.
func TestDiscoverMasksFile_InStartDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), "")
	masks := filepath.Join(dir, MasksFileName)
	writeFile(t, masks, "version: 1\n")

	got, err := DiscoverMasksFile(dir)
	if err != nil {
		t.Fatalf("DiscoverMasksFile: %v", err)
	}
	if got != masks {
		t.Fatalf("got %q, want %q", got, masks)
	}
}

// TestDiscoverMasksFile_InParentDir exercises the upward-walk branch
// of the discovery algorithm. The masks file is placed at the root
// of the temporary directory tree, and DiscoverMasksFile is invoked
// from a nested subdirectory. The expected behavior is that the
// algorithm walks up one level, finds the file, and returns its
// absolute path. A .git directory is once again planted at the
// temporary root to act as a hard upper bound on the search.
func TestDiscoverMasksFile_InParentDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "")
	masks := filepath.Join(root, MasksFileName)
	writeFile(t, masks, "version: 1\n")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverMasksFile(sub)
	if err != nil {
		t.Fatalf("DiscoverMasksFile: %v", err)
	}
	if got != masks {
		t.Fatalf("got %q, want %q", got, masks)
	}
}

// TestDiscoverMasksFile_StopsAtGitBoundary documents and locks in
// the intentional design choice that DiscoverMasksFile must never
// cross a repository boundary while walking upward. Concretely, if
// the algorithm encounters a directory that contains a .git entry
// before it has found the masks file, it must immediately return an
// empty path and a nil error, signalling "no policy file applicable
// to this repository". The test arranges a layout in which a masks
// file exists outside of an inner repository while the inner
// repository contains a .git directory positioned strictly between
// the start directory and the masks file, and verifies that the
// returned path is empty.
func TestDiscoverMasksFile_StopsAtGitBoundary(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, MasksFileName), "version: 1\n")
	inner := filepath.Join(root, "sub", "inner")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "sub", ".git", "HEAD"), "")

	got, err := DiscoverMasksFile(inner)
	if err != nil {
		t.Fatalf("DiscoverMasksFile: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty (stopped at .git)", got)
	}
}

// TestDiscoverMasksFile_NotFound covers the case in which no masks
// file exists anywhere along the upward search path. The expected
// behavior is that DiscoverMasksFile returns an empty string and a
// nil error, since "no policy file configured" is a legitimate
// state rather than a hard failure. A .git directory is planted at
// the temporary root in order to terminate the walk deterministically
// regardless of the host filesystem layout above the temporary
// directory.
func TestDiscoverMasksFile_NotFound(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".git", "HEAD"), "")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverMasksFile(sub)
	if err != nil {
		t.Fatalf("DiscoverMasksFile: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
