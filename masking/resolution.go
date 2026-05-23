package masking

import (
	"errors"
	"path/filepath"
)

// consumers translate to their own user-facing wording
var ErrNoMasksFile = errors.New("no masks file resolved")

// CLI > profile > discovery. Cwd anchors relative FlagFile/ProfileFile and
// is the discovery start dir; absolutize when the two have different anchors.
type Resolution struct {
	FlagFile        string
	ProfileFile     string
	FlagPolicies    []string
	ProfilePolicies []string
	DatabaseID      string
	Cwd             string
	Disabled        bool
}

// Disabled → ("", nil, nil); no source → ErrNoMasksFile
func (r Resolution) Resolve() (path string, policies []string, err error) {
	if r.Disabled {
		return "", nil, nil
	}
	path = r.resolvePath(r.FlagFile)
	if path == "" {
		path = r.resolvePath(r.ProfileFile)
	}
	if path == "" {
		discovered, derr := DiscoverMasksFile(r.Cwd)
		if derr != nil {
			return "", nil, derr
		}
		path = discovered
	}
	if path == "" {
		return "", nil, ErrNoMasksFile
	}
	policies = r.FlagPolicies
	if len(policies) == 0 {
		policies = r.ProfilePolicies
	}
	return path, policies, nil
}

// Disabled → (nil, nil); no source → (nil, ErrNoMasksFile)
func (r Resolution) Load() (*Policy, error) {
	path, policies, err := r.Resolve()
	if err != nil || path == "" {
		return nil, err
	}
	return Load(path, r.DatabaseID, policies)
}

func (r Resolution) resolvePath(p string) string {
	if p == "" || filepath.IsAbs(p) || r.Cwd == "" {
		return p
	}
	return filepath.Join(r.Cwd, p)
}
