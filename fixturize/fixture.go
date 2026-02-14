package fixturize

import (
	"encoding/json"
	"time"
)

type (
	Fixture struct {
		Meta       FixtureMeta              `json:"meta"`
		TableOrder []string                 `json:"table_order,omitempty"`
		Tables     map[string]*FixtureTable `json:"tables"`
	}

	FixtureMeta struct {
		ExtractedAt     time.Time `json:"extracted_at"`
		Root            string    `json:"root"`
		MasksApplied    []string  `json:"masks_applied,omitempty"`
		UntouchedTables []string  `json:"untouched_tables,omitempty"`
	}

	FixtureTable struct {
		Columns         []string `json:"columns"`
		Rows            [][]any  `json:"rows"`
		IdentityColumns bool     `json:"identity_columns,omitempty"`
	}
)

func NewFixture(root string, masks []string) *Fixture {
	return &Fixture{
		Meta: FixtureMeta{
			ExtractedAt:  time.Now().UTC(),
			Root:         root,
			MasksApplied: masks,
		},
		Tables: make(map[string]*FixtureTable),
	}
}

func (f *Fixture) AddTable(name string, columns []string, rows [][]any) {
	f.Tables[name] = &FixtureTable{
		Columns: columns,
		Rows:    rows,
	}
}

func (f *Fixture) SetUntouchedTables(tables []string) {
	f.Meta.UntouchedTables = tables
}

func (f *Fixture) ToJSON() ([]byte, error) {
	return json.MarshalIndent(f, "", "  ")
}

func LoadFixture(data []byte) (*Fixture, error) {
	var f Fixture
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
