package fixturize

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestLoadProfile_PureParse_NoMasksFileIO(t *testing.T) {
	// LoadProfile must not touch the filesystem for masks; even with a bogus
	// masks_file path, parse should succeed. The mistake of resolving masks at
	// load time is what the resolve_masks adapter exists to fix.
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	writeFile(t, profilePath, `
database_id: app
masks_file: ./does-not-exist.yml
extract:
  root: "users WHERE id = 1"
  mask_policies: [pii]
`)

	p, err := LoadProfile(profilePath)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.MasksFile != "./does-not-exist.yml" {
		t.Errorf("MasksFile = %q, want raw value", p.MasksFile)
	}
	if p.Path() != profilePath {
		t.Errorf("Path() = %q, want %q", p.Path(), profilePath)
	}
}

func TestLoadProfile_InlineMasksSurvive(t *testing.T) {
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
	if !reflect.DeepEqual(p.Masks["pii"], []string{"users.email='masked'"}) {
		t.Errorf("Masks[pii] = %v", p.Masks["pii"])
	}
}
