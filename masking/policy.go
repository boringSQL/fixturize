package masking

import (
	"fmt"
	"strings"
)

type Policy struct {
	dbID        string
	qualified   map[string]struct{} // "schema.table.column"
	unqualified map[string]struct{} // "table.column", any schema
}

func Load(path, databaseID string, policyNames []string) (*Policy, error) {
	f, err := LoadSharedMasks(path)
	if err != nil {
		return nil, err
	}

	db, ok := f.Databases[databaseID]
	if !ok {
		return nil, fmt.Errorf("masks file %s has no entry for database_id %q", path, databaseID)
	}

	p := &Policy{
		dbID:        databaseID,
		qualified:   map[string]struct{}{},
		unqualified: map[string]struct{}{},
	}

	selected, err := selectColumns(db, policyNames)
	if err != nil {
		return nil, err
	}
	for _, key := range selected {
		p.add(key)
	}
	return p, nil
}

// qualified wins over unqualified
func (p *Policy) IsSensitive(schema, table, column string) bool {
	if p == nil {
		return false
	}
	if _, ok := p.qualified[schema+"."+table+"."+column]; ok {
		return true
	}
	_, ok := p.unqualified[table+"."+column]
	return ok
}

func (p *Policy) DatabaseID() string {
	if p == nil {
		return ""
	}
	return p.dbID
}

// malformed keys silently skipped — no logging from library code
func (p *Policy) add(key string) {
	switch strings.Count(key, ".") {
	case 2:
		p.qualified[key] = struct{}{}
	case 1:
		p.unqualified[key] = struct{}{}
	}
}

// empty policyNames = all listed columns; else union by tag intersection
func selectColumns(db SharedDatabase, policyNames []string) ([]string, error) {
	if len(policyNames) == 0 {
		keys := make([]string, 0, len(db.Columns))
		for key := range db.Columns {
			keys = append(keys, key)
		}
		return keys, nil
	}

	tags := map[string]struct{}{}
	for _, name := range policyNames {
		pol, ok := db.Policies[name]
		if !ok {
			return nil, fmt.Errorf("masks file has no policy %q", name)
		}
		for _, t := range pol.IncludeTags {
			tags[t] = struct{}{}
		}
	}

	var keys []string
	for key, col := range db.Columns {
		for _, t := range col.Tags {
			if _, ok := tags[t]; ok {
				keys = append(keys, key)
				break
			}
		}
	}
	return keys, nil
}
