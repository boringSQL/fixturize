package masking

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

const minimalMasks = `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`

// Disabled short-circuits everything, even a populated ProfileFile.
func TestResolution_Disabled(t *testing.T) {
	r := Resolution{Disabled: true, ProfileFile: "/nope"}
	p, err := r.Load()
	if err != nil || p != nil {
		t.Errorf("disabled should return (nil, nil); got (%v, %v)", p, err)
	}
}

// FlagFile beats ProfileFile. two files on disk with distinct exprs let us
// tell which one was picked; we go through Load() so a textually-selected
// but unreadable path would also be caught.
func TestResolution_FlagWinsOverProfile(t *testing.T) {
	dir := t.TempDir()
	flagPath := filepath.Join(dir, "flag.yml")
	profilePath := filepath.Join(dir, "profile.yml")
	writeYAML(t, flagPath, minimalMasks)
	writeYAML(t, profilePath, `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'profile'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	r := Resolution{
		FlagFile:        flagPath,
		ProfileFile:     profilePath,
		ProfilePolicies: []string{"pii"},
		DatabaseID:      "app",
	}
	p, err := r.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	exprs := p.Expressions()
	if len(exprs) != 1 || exprs[0] != "users.email='x'" {
		t.Errorf("expected flag file selected, got %v", exprs)
	}
}

// relative ProfileFile is joined onto Cwd. this is why Cwd is an explicit
// field rather than implicit — callers pick the anchor per source.
func TestResolution_RelativePathJoinsCwd(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, "policy.yml"), minimalMasks)
	r := Resolution{
		ProfileFile:     "policy.yml",
		ProfilePolicies: []string{"pii"},
		DatabaseID:      "app",
		Cwd:             dir,
	}
	if _, err := r.Load(); err != nil {
		t.Errorf("relative path should resolve against Cwd: %v", err)
	}
}

// no flag, no profile → fall through to DiscoverMasksFile(Cwd).
// multi-level walk-up + .git boundary are covered in the fixturize tests.
func TestResolution_DiscoveryFallback(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, filepath.Join(dir, MasksFileName), minimalMasks)
	r := Resolution{DatabaseID: "app", Cwd: dir}
	if _, err := r.Load(); err != nil {
		t.Errorf("discovery should find file in Cwd: %v", err)
	}
}

// nothing supplied + .git stops discovery → ErrNoMasksFile (sentinel, not
// a generic os error). callers translate this to their own wording.
func TestResolution_NoSourceReturnsErrNoMasksFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := Resolution{Cwd: dir}
	_, err := r.Load()
	if !errors.Is(err, ErrNoMasksFile) {
		t.Errorf("expected ErrNoMasksFile, got %v", err)
	}
}

// FlagPolicies wholesale replaces ProfilePolicies (no merge). a merge would
// stop a CLI user from narrowing the active set below what the profile declared.
func TestResolution_FlagPoliciesOverrideProfilePolicies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yml")
	writeYAML(t, path, `
version: 1
databases:
  app:
    columns:
      a.col: { expr: "1", tags: [pii] }
      b.col: { expr: "2", tags: [financial] }
    policies:
      pii:       { include_tags: [pii] }
      financial: { include_tags: [financial] }
`)
	r := Resolution{
		ProfileFile:     path,
		ProfilePolicies: []string{"pii"},
		FlagPolicies:    []string{"financial"},
		DatabaseID:      "app",
	}
	p, err := r.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	exprs := p.Expressions()
	if len(exprs) != 1 || exprs[0] != "b.col=2" {
		t.Errorf("flag policies should win; got %v", exprs)
	}
}
