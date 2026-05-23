package fixturize

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boringsql/fixturize/masking"
)

type ResolveMasksOptions struct {
	FlagFile     string   // --masks-file
	FlagPolicies []string // --mask-policy
	Disabled     bool     // --no-masks
	Cwd          string   // anchor for relative FlagFile, discovery start when no profile
}

// merges: file-selected exprs + inline profile.masks for mask_policies names not in the file + raw extract.masks
func ResolveMasks(profile *Profile, opts ResolveMasksOptions) ([]string, error) {
	var (
		inlineMasks  map[string][]string
		maskPolicies []string
		extractRaw   []string
		profileFile  string
		profileDir   string
		databaseID   string
	)
	if profile != nil {
		inlineMasks = profile.Masks
		maskPolicies = profile.Extract.MaskPolicies
		extractRaw = profile.Extract.Masks
		databaseID = profile.DatabaseID
		if profile.path != "" {
			profileDir = filepath.Dir(profile.path)
		}
		if profile.MasksFile != "" {
			profileFile = profile.MasksFile
			if !filepath.IsAbs(profileFile) && profileDir != "" {
				profileFile = filepath.Join(profileDir, profileFile)
			}
		}
	}

	flagFile := opts.FlagFile
	if flagFile != "" && !filepath.IsAbs(flagFile) && opts.Cwd != "" {
		flagFile = filepath.Join(opts.Cwd, flagFile)
	}

	anchor := profileDir
	if anchor == "" {
		anchor = opts.Cwd
	}

	res := masking.Resolution{
		FlagFile:        flagFile,
		ProfileFile:     profileFile,
		FlagPolicies:    opts.FlagPolicies,
		ProfilePolicies: maskPolicies,
		DatabaseID:      databaseID,
		Cwd:             anchor,
		Disabled:        opts.Disabled,
	}

	if opts.Disabled {
		return nil, nil
	}

	path, policies, err := res.Resolve()
	if err != nil && !errors.Is(err, masking.ErrNoMasksFile) {
		return nil, err
	}

	var (
		fileExprs        []string
		filePolicyNames  map[string]struct{}
		databaseIDInFile bool
	)
	// explicit masks file (flag or profile) without database_id is a config bug — error loudly.
	// a discovered file (walk-up) is opportunistic context: warn and skip, don't force opt-out.
	if path != "" && databaseID == "" {
		explicit := flagFile != "" || profileFile != ""
		if explicit {
			return nil, fmt.Errorf("masks file %s configured but profile/flag did not set database_id", path)
		}
		fmt.Fprintf(os.Stderr, "Warning: discovered masks file %s but profile has no database_id — skipping (no masking applied)\n", path)
		path = ""
	}
	if path != "" {
		shared, err := masking.LoadSharedMasks(path)
		if err != nil {
			return nil, err
		}
		db, ok := shared.Databases[databaseID]
		if !ok {
			ids := make([]string, 0, len(shared.Databases))
			for id := range shared.Databases {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			return nil, fmt.Errorf("database_id %q not found in %s (available: %s)", databaseID, path, strings.Join(ids, ", "))
		}
		databaseIDInFile = true
		filePolicyNames = make(map[string]struct{}, len(db.Policies))
		for name := range db.Policies {
			filePolicyNames[name] = struct{}{}
		}

		// inline-only names are deferred to the lookup below
		var fileSelected []string
		for _, name := range policies {
			if _, ok := db.Policies[name]; ok {
				fileSelected = append(fileSelected, name)
			}
		}
		// asked for specific names, none in file → emit nothing (don't fall through to "all columns")
		if len(policies) > 0 && len(fileSelected) == 0 {
			fileExprs = nil
		} else {
			pol, err := masking.Load(path, databaseID, fileSelected)
			if err != nil {
				return nil, err
			}
			fileExprs = pol.Expressions()
		}
	}

	var inlineExprs []string
	for _, name := range maskPolicies {
		if databaseIDInFile {
			if _, ok := filePolicyNames[name]; ok {
				continue
			}
		}
		exprs, ok := inlineMasks[name]
		if !ok {
			return nil, fmt.Errorf("undefined mask policy: %s", name)
		}
		inlineExprs = append(inlineExprs, exprs...)
	}

	result := make([]string, 0, len(fileExprs)+len(inlineExprs)+len(extractRaw))
	result = append(result, fileExprs...)
	result = append(result, inlineExprs...)
	result = append(result, extractRaw...)
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}
