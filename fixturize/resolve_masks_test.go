package fixturize

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func mustLoad(t *testing.T, path string) *Profile {
	t.Helper()
	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	return p
}

// inline `masks` map referenced via `extract.mask_policies` — no file, no discovery
func TestResolveMasks_InlineOnly(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
masks:
  pii:
    - "users.email='masked'"
extract:
  root: "users WHERE id = 1"
  mask_policies: [pii]
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"users.email='masked'"}) {
		t.Errorf("got %v", got)
	}
}

// canonical end-to-end: profile points at a shared masks_file via database_id,
// asks for two named policies → resolver returns the deterministic union of
// selected columns. sort order matters for golden-file diffing downstream.
func TestResolveMasks_SharedHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email:    { expr: "'x@x'", tags: [pii] }
      users.phone:    { expr: "'000'", tags: [pii, contact] }
      orders.total:   { expr: "0",     tags: [financial] }
    policies:
      pii:       { include_tags: [pii] }
      financial: { include_tags: [financial] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./data-masking-policy.yml
extract:
  root: "users WHERE id = 1"
  mask_policies: [pii, financial]
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	want := []string{"orders.total=0", "users.email='x@x'", "users.phone='000'"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// a column with multiple tags must appear once, not per matching policy
func TestResolveMasks_TagOverlap(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.bank: { expr: "'0'", tags: [pii, financial] }
    policies:
      pii:       { include_tags: [pii] }
      financial: { include_tags: [financial] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./data-masking-policy.yml
extract:
  root: "users WHERE id = 1"
  mask_policies: [pii]
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	// Union under both names; tag overlap means the same column shows up once.
	if !reflect.DeepEqual(got, []string{"users.bank='0'"}) {
		t.Errorf("got %v", got)
	}
}

// behavior change from pre-refactor: inline `masks` and shared-file policies
// can coexist under one profile. file-backed names take the shared path;
// remaining names fall through to inline-map lookup. previously this errored.
func TestResolveMasks_InlineAndSharedCoexist(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./data-masking-policy.yml
masks:
  extra:
    - "orders.note='masked'"
extract:
  root: "users WHERE id = 1"
  mask_policies: [pii, extra]
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	want := []string{"users.email='x'", "orders.note='masked'"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// error must name the bogus id AND list valid ones (sorted), so a typo in
// database_id is actionable without opening the masks file.
func TestResolveMasks_MissingDatabaseID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 1
databases:
  auth: { columns: {}, policies: {} }
  app:  { columns: {}, policies: {} }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: bogus
masks_file: ./data-masking-policy.yml
extract:
  root: "users WHERE id = 1"
`)
	_, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err == nil {
		t.Fatal("expected error for missing database_id")
	}
	msg := err.Error()
	for _, want := range []string{"bogus", "app", "auth"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

// version gate surfaces through ResolveMasks, not just LoadSharedMasks
func TestResolveMasks_UnknownVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 2
databases: {}
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./data-masking-policy.yml
extract:
  root: "users WHERE id = 1"
`)
	_, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("expected version error, got %v", err)
	}
}

// no masks_file, no flag → walk up from profile dir to find data-masking-policy.yml
func TestResolveMasks_WalkUpDiscovery(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	writeFile(t, filepath.Join(root, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(nested, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
extract:
  root: "users WHERE id = 1"
  mask_policies: [pii]
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"users.email='x'"}) {
		t.Errorf("walk-up discovery failed: %v", got)
	}
}

// walk-up must stop at .git. without this, a profile in a nested repo or
// vendored dep could silently pick up the outer project's masks config.
func TestResolveMasks_GitBoundaryStopsWalkUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	inner := filepath.Join(root, "inner")
	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(inner, "sub", "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
extract:
  root: "users WHERE id = 1"
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if got != nil {
		t.Errorf(".git boundary should have stopped walk-up; got %v", got)
	}
}

// no masks anywhere → (nil, nil); masking is optional
func TestResolveMasks_NoFileNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
extract:
  root: "users WHERE id = 1"
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// explicitly configured masks_file without database_id is a config bug —
// error loudly so unmasked fixtures don't ship silently.
func TestResolveMasks_ExplicitFileNoDatabaseID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "masks.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
masks_file: masks.yml
extract:
  root: "users WHERE id = 1"
`)
	_, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err == nil || !strings.Contains(err.Error(), "database_id") {
		t.Fatalf("expected database_id error, got %v", err)
	}
}

// same, but via --masks-file flag instead of profile field.
func TestResolveMasks_FlagFileNoDatabaseID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "masks.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
extract:
  root: "users WHERE id = 1"
`)
	_, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{
		FlagFile: filepath.Join(dir, "masks.yml"),
	})
	if err == nil || !strings.Contains(err.Error(), "database_id") {
		t.Fatalf("expected database_id error, got %v", err)
	}
}

// monorepo case: walk-up finds an ancestor masks file but the profile
// doesn't set database_id → silently skip, don't force the user to opt out.
func TestResolveMasks_DiscoveredFileNoDatabaseID(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(root, "sub", "profile.yaml")
	writeFile(t, profilePath, `
extract:
  root: "users WHERE id = 1"
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (no opt-in via database_id); got %v", got)
	}
}

// --no-masks suppresses everything: file, inline, raw extract.masks
func TestResolveMasks_Disabled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'x'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./data-masking-policy.yml
masks:
  extra:
    - "orders.note='masked'"
extract:
  mask_policies: [pii, extra]
  masks:
    - "users.name='nope'"
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{Disabled: true})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if got != nil {
		t.Errorf("--no-masks should suppress everything; got %v", got)
	}
}

// --masks-file wins over profile.masks_file unconditionally
func TestResolveMasks_FlagOverridesProfile(t *testing.T) {
	dir := t.TempDir()
	// Two distinct files; profile points at the first, flag at the second.
	writeFile(t, filepath.Join(dir, "profile-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'profile'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	writeFile(t, filepath.Join(dir, "flag-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email: { expr: "'flag'", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./profile-policy.yml
extract:
  mask_policies: [pii]
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{
		FlagFile: filepath.Join(dir, "flag-policy.yml"),
	})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"users.email='flag'"}) {
		t.Errorf("flag should override profile; got %v", got)
	}
}

// --mask-policy replaces (not merges) profile.extract.mask_policies. merging
// would make it impossible to narrow the active set from the CLI.
func TestResolveMasks_FlagPoliciesOverrideProfile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      users.email:  { expr: "'x'", tags: [pii] }
      orders.total: { expr: "0",   tags: [financial] }
    policies:
      pii:       { include_tags: [pii] }
      financial: { include_tags: [financial] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./data-masking-policy.yml
extract:
  mask_policies: [pii]
`)
	got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{
		FlagPolicies: []string{"financial"},
	})
	if err != nil {
		t.Fatalf("ResolveMasks: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"orders.total=0"}) {
		t.Errorf("flag policies should win; got %v", got)
	}
}

// run 5x against the same profile; first must be sorted, all must match.
// guards against accidental map iteration without an explicit sort.
func TestResolveMasks_DeterministicOrdering(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "data-masking-policy.yml"), `
version: 1
databases:
  app:
    columns:
      a.col: { expr: "1", tags: [pii] }
      z.col: { expr: "1", tags: [pii] }
      m.col: { expr: "1", tags: [pii] }
      b.col: { expr: "1", tags: [pii] }
    policies:
      pii: { include_tags: [pii] }
`)
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./data-masking-policy.yml
extract:
  mask_policies: [pii]
`)
	var first []string
	for i := 0; i < 5; i++ {
		got, err := ResolveMasks(mustLoad(t, profilePath), ResolveMasksOptions{})
		if err != nil {
			t.Fatalf("ResolveMasks: %v", err)
		}
		if i == 0 {
			first = got
			if !sort.StringsAreSorted(first) {
				t.Errorf("not sorted: %v", first)
			}
			continue
		}
		if !reflect.DeepEqual(got, first) {
			t.Errorf("non-deterministic: run %d got %v, want %v", i, got, first)
		}
	}
}
