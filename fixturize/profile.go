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
		Masks      map[string][]string `yaml:"masks"`
		Extract    ExtractProfile      `yaml:"extract"`
		Apply      ApplyProfile        `yaml:"apply"`
	}

	ExtractProfile struct {
		Root                string   `yaml:"root"`
		Output              string   `yaml:"output"`
		Format              string   `yaml:"format"`
		Limit               int      `yaml:"limit"`
		Depth               int      `yaml:"depth"`
		StatementTimeout    int      `yaml:"statement_timeout"`
		Transaction         bool     `yaml:"transaction"`
		OnConflictDoNothing bool     `yaml:"on_conflict_do_nothing"`
		Include             []string `yaml:"include"`
		Exclude             []string `yaml:"exclude"`
		MaskPolicies        []string `yaml:"mask_policies"`
		Masks               []string `yaml:"masks"`
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

	rawContent := string(data)
	if err := validateEnvVars(rawContent); err != nil {
		return nil, err
	}

	content := interpolateEnvVars(rawContent)

	var profile Profile
	if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
		return nil, fmt.Errorf("failed to parse profile: %w", err)
	}

	if err := profile.Validate(); err != nil {
		return nil, err
	}

	return &profile, nil
}

func interpolateEnvVars(content string) string {
	return os.Expand(content, os.Getenv)
}

func (p *Profile) ResolveMasks() []string {
	var result []string

	for _, policyName := range p.Extract.MaskPolicies {
		if masks, ok := p.Masks[policyName]; ok {
			result = append(result, masks...)
		}
	}

	result = append(result, p.Extract.Masks...)

	return result
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

func (p *Profile) Validate() error {
	for _, policyName := range p.Extract.MaskPolicies {
		if _, ok := p.Masks[policyName]; !ok {
			return fmt.Errorf("undefined mask policy: %s", policyName)
		}
	}

	return nil
}
