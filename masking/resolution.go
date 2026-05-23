package masking

import "errors"

// consumers translate to their own user-facing wording
var ErrNoMasksFile = errors.New("no masks file resolved")

type Resolution struct {
	FlagFile        string
	ProfileFile     string
	FlagPolicies    []string
	ProfilePolicies []string
	DatabaseID      string
	Cwd             string
	Disabled        bool
}

// CLI > profile > discovery; Disabled → (nil, nil)
func (r Resolution) Load() (*Policy, error) {
	if r.Disabled {
		return nil, nil
	}
	path := r.FlagFile
	if path == "" {
		path = r.ProfileFile
	}
	if path == "" {
		if d, _ := DiscoverMasksFile(r.Cwd); d != "" {
			path = d
		}
	}
	if path == "" {
		return nil, ErrNoMasksFile
	}
	policies := r.FlagPolicies
	if len(policies) == 0 {
		policies = r.ProfilePolicies
	}
	return Load(path, r.DatabaseID, policies)
}
