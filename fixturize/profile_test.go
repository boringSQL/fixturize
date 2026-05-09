package fixturize

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadProfile_InlineOnly_BackwardCompat(t *testing.T) {
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

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	want := []string{"users.email='masked'"}
	if !reflect.DeepEqual(p.Masks["pii"], want) {
		t.Errorf("Masks[pii] = %v, want %v", p.Masks["pii"], want)
	}
}

func TestLoadProfile_SharedHappyPath(t *testing.T) {
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

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}

	wantPII := []string{"users.email='x@x'", "users.phone='000'"}
	if !reflect.DeepEqual(p.Masks["pii"], wantPII) {
		t.Errorf("Masks[pii] = %v, want %v", p.Masks["pii"], wantPII)
	}
	wantFin := []string{"orders.total=0"}
	if !reflect.DeepEqual(p.Masks["financial"], wantFin) {
		t.Errorf("Masks[financial] = %v, want %v", p.Masks["financial"], wantFin)
	}
}

func TestLoadProfile_TagOverlap(t *testing.T) {
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
`)

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	for _, name := range []string{"pii", "financial"} {
		if got, want := p.Masks[name], []string{"users.bank='0'"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Masks[%s] = %v, want %v", name, got, want)
		}
	}
}

func TestLoadProfile_InlineAndSharedMerge(t *testing.T) {
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
`)

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if got := p.Masks["pii"]; !reflect.DeepEqual(got, []string{"users.email='x'"}) {
		t.Errorf("Masks[pii] = %v", got)
	}
	if got := p.Masks["extra"]; !reflect.DeepEqual(got, []string{"orders.note='masked'"}) {
		t.Errorf("Masks[extra] = %v", got)
	}
}

func TestLoadProfile_PolicyConflict(t *testing.T) {
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
  pii:
    - "users.email='inline'"
extract:
  root: "users WHERE id = 1"
`)

	_, err := LoadProfile(profilePath)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "pii") {
		t.Errorf("error should name conflicting policy: %v", err)
	}
}

func TestLoadProfile_MissingDatabaseID(t *testing.T) {
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

	_, err := LoadProfile(profilePath)
	if err == nil {
		t.Fatal("expected error for missing database_id")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bogus") || !strings.Contains(msg, "app") || !strings.Contains(msg, "auth") {
		t.Errorf("error should list bogus + available IDs: %v", err)
	}
}

func TestLoadProfile_UnknownVersion(t *testing.T) {
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

	_, err := LoadProfile(profilePath)
	if err == nil {
		t.Fatal("expected version error")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should mention version: %v", err)
	}
}

func TestLoadProfile_WalkUpDiscovery(t *testing.T) {
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
`)

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if got := p.Masks["pii"]; !reflect.DeepEqual(got, []string{"users.email='x'"}) {
		t.Errorf("walk-up discovery failed: Masks[pii] = %v", got)
	}
}

func TestLoadProfile_GitBoundaryStopsWalkUp(t *testing.T) {
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
	// .git boundary inside `inner`, profile lives below it
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

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if _, ok := p.Masks["pii"]; ok {
		t.Errorf(".git boundary should have stopped walk-up; got Masks[pii]=%v", p.Masks["pii"])
	}
}

func TestLoadProfile_NoFileNoDatabaseID_NoOp(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
extract:
  root: "users WHERE id = 1"
`)

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if len(p.Masks) != 0 {
		t.Errorf("expected no masks, got %v", p.Masks)
	}
}

func TestLoadProfile_DeterministicOrdering(t *testing.T) {
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
  root: "users WHERE id = 1"
`)

	var first []string
	for i := 0; i < 5; i++ {
		p, err := LoadProfile(profilePath)
		if err != nil {
			t.Fatalf("LoadProfile: %v", err)
		}
		got := p.Masks["pii"]
		if i == 0 {
			first = got
			if !sort.StringsAreSorted(first) {
				t.Errorf("output not sorted: %v", first)
			}
			continue
		}
		if !reflect.DeepEqual(got, first) {
			t.Errorf("non-deterministic ordering: run %d got %v, want %v", i, got, first)
		}
	}
}
