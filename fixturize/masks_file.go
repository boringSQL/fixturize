package fixturize

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const MasksFileName = "data-masking-policy.yml"

type (
	SharedMasksFile struct {
		Version   int                       `yaml:"version"`
		Databases map[string]SharedDatabase `yaml:"databases"`
	}

	SharedDatabase struct {
		Columns  map[string]SharedColumn `yaml:"columns"`
		Policies map[string]SharedPolicy `yaml:"policies"`
	}

	SharedColumn struct {
		Expr string   `yaml:"expr"`
		Tags []string `yaml:"tags"`
	}

	SharedPolicy struct {
		IncludeTags []string `yaml:"include_tags"`
	}
)

func LoadSharedMasks(path string) (*SharedMasksFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read masks file %s: %w", path, err)
	}

	var f SharedMasksFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to parse masks file %s: %w", path, err)
	}

	if f.Version != 1 {
		return nil, fmt.Errorf("unsupported masks file version %d in %s (expected 1)", f.Version, path)
	}

	return &f, nil
}

// DiscoverMasksFile walks up from startDir looking for data-masking-policy.yml
func DiscoverMasksFile(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve start dir: %w", err)
	}

	for {
		candidate := filepath.Join(dir, MasksFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}

		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return "", nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}
