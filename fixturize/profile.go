package fixturize

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type (
	Profile struct {
		Connection string              `yaml:"connection"`
		Schema     string              `yaml:"schema"`
		DatabaseID string              `yaml:"database_id"`
		MasksFile  string              `yaml:"masks_file"`
		Masks      map[string][]string `yaml:"masks"`
		Extract    ExtractProfile      `yaml:"extract"`
		Apply      ApplyProfile        `yaml:"apply"`

		// anchor for relative MasksFile and discovery start; "" for in-memory
		path string
	}

	ExtractProfile struct {
		Root                string            `yaml:"root"`
		Seed                string            `yaml:"seed"`
		Output              string            `yaml:"output"`
		Format              string            `yaml:"format"`
		Limit               int               `yaml:"limit"`
		Depth               int               `yaml:"depth"`
		StatementTimeout    int               `yaml:"statement_timeout"`
		Transaction         bool              `yaml:"transaction"`
		OnConflictDoNothing bool              `yaml:"on_conflict_do_nothing"`
		Include             []string          `yaml:"include"`
		Exclude             []string          `yaml:"exclude"`
		MaskPolicies        []string          `yaml:"mask_policies"`
		Masks               []string          `yaml:"masks"`
		Filters             map[string]string `yaml:"filters"`
	}

	ApplyProfile struct {
		Force           bool `yaml:"force"`
		DisableTriggers bool `yaml:"disable_triggers"`
		SyncSequences   bool `yaml:"sync_sequences"`
	}
)

func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile: %w", err)
	}

	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}

	if err := expandConnectionEnvVars(&profile); err != nil {
		return nil, err
	}

	profile.path = path
	return &profile, nil
}

func (p *Profile) Path() string {
	if p == nil {
		return ""
	}
	return p.path
}

func expandConnectionEnvVars(p *Profile) error {
	if p.Connection == "" {
		return nil
	}
	if err := validateEnvVars(p.Connection); err != nil {
		return err
	}
	p.Connection = os.Expand(p.Connection, os.Getenv)
	return nil
}

func validateEnvVars(content string) error {
	envVarPattern := regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)
	matches := envVarPattern.FindAllStringSubmatch(content, -1)

	var unresolved []string
	for _, match := range matches {
		varName := match[1]
		if varName == "" {
			varName = match[2]
		}
		if os.Getenv(varName) == "" {
			unresolved = append(unresolved, match[0])
		}
	}

	if len(unresolved) > 0 {
		return fmt.Errorf("unresolved environment variable(s): %s", strings.Join(unresolved, ", "))
	}

	return nil
}
