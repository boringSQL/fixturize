package fixturize

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	}

	ExtractProfile struct {
		Root                string   `yaml:"root"`
		Seed                string   `yaml:"seed"`
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

	if err := mergeSharedMasks(&profile, path); err != nil {
		return nil, err
	}

	if err := profile.Validate(); err != nil {
		return nil, err
	}

	return &profile, nil
}

func mergeSharedMasks(p *Profile, profilePath string) error {
	profileDir := filepath.Dir(profilePath)

	masksPath, err := resolveMasksPath(p.MasksFile, profileDir)
	if err != nil {
		return err
	}
	if masksPath == "" {
		return nil
	}

	shared, err := LoadSharedMasks(masksPath)
	if err != nil {
		return err
	}

	if p.DatabaseID == "" {
		return nil
	}

	db, ok := shared.Databases[p.DatabaseID]
	if !ok {
		ids := make([]string, 0, len(shared.Databases))
		for id := range shared.Databases {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return fmt.Errorf("database_id %q not found in %s (available: %s)", p.DatabaseID, masksPath, strings.Join(ids, ", "))
	}

	if p.Masks == nil {
		p.Masks = map[string][]string{}
	}

	for name, policy := range db.Policies {
		if _, exists := p.Masks[name]; exists {
			return fmt.Errorf("mask policy %q defined both inline and in %s", name, masksPath)
		}
		p.Masks[name] = expandPolicy(policy, db.Columns)
	}

	return nil
}

func resolveMasksPath(explicit, profileDir string) (string, error) {
	if explicit != "" {
		if filepath.IsAbs(explicit) {
			return explicit, nil
		}
		return filepath.Join(profileDir, explicit), nil
	}
	return DiscoverMasksFile(profileDir)
}

func expandPolicy(policy SharedPolicy, columns map[string]SharedColumn) []string {
	tagSet := make(map[string]struct{}, len(policy.IncludeTags))
	for _, t := range policy.IncludeTags {
		tagSet[t] = struct{}{}
	}

	keys := make([]string, 0, len(columns))
	for key, col := range columns {
		for _, t := range col.Tags {
			if _, ok := tagSet[t]; ok {
				keys = append(keys, key)
				break
			}
		}
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s=%s", key, columns[key].Expr))
	}
	return out
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
