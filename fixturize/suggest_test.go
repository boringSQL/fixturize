package fixturize

import (
	"testing"
)

func TestSuggestRootTables(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.organizations": {
				Schema:  "public",
				Name:    "organizations",
				Columns: map[string]*ColumnInfo{"id": {}, "name": {}, "domain": {}, "created_at": {}, "plan": {}},
			},
			"public.users": {
				Schema:  "public",
				Name:    "users",
				Columns: map[string]*ColumnInfo{"id": {}, "email": {}, "org_id": {}, "name": {}},
				ForeignKeys: []*ForeignKeyInfo{
					{ColumnName: "org_id", ReferencedTable: "public.organizations", ReferencedColumn: "id"},
				},
			},
			"public.projects": {
				Schema:  "public",
				Name:    "projects",
				Columns: map[string]*ColumnInfo{"id": {}, "name": {}, "user_id": {}, "org_id": {}},
				ForeignKeys: []*ForeignKeyInfo{
					{ColumnName: "user_id", ReferencedTable: "public.users", ReferencedColumn: "id"},
					{ColumnName: "org_id", ReferencedTable: "public.organizations", ReferencedColumn: "id"},
				},
			},
			"public.tasks": {
				Schema:  "public",
				Name:    "tasks",
				Columns: map[string]*ColumnInfo{"id": {}, "title": {}, "project_id": {}, "assignee_id": {}},
				ForeignKeys: []*ForeignKeyInfo{
					{ColumnName: "project_id", ReferencedTable: "public.projects", ReferencedColumn: "id"},
					{ColumnName: "assignee_id", ReferencedTable: "public.users", ReferencedColumn: "id"},
				},
			},
			// Leaf table: only outgoing FKs, no children
			"public.comments": {
				Schema:  "public",
				Name:    "comments",
				Columns: map[string]*ColumnInfo{"id": {}, "body": {}, "task_id": {}, "user_id": {}},
				ForeignKeys: []*ForeignKeyInfo{
					{ColumnName: "task_id", ReferencedTable: "public.tasks", ReferencedColumn: "id"},
					{ColumnName: "user_id", ReferencedTable: "public.users", ReferencedColumn: "id"},
				},
			},
			// Lookup table: few columns, no children
			"public.statuses": {
				Schema:  "public",
				Name:    "statuses",
				Columns: map[string]*ColumnInfo{"id": {}, "name": {}},
			},
		},
	}

	candidates := schema.SuggestRootTables()

	t.Run("leaf table excluded", func(t *testing.T) {
		for _, c := range candidates {
			if c.Table == "public.comments" {
				t.Error("leaf table (comments) should be excluded")
			}
		}
	})

	t.Run("lookup table excluded", func(t *testing.T) {
		for _, c := range candidates {
			if c.Table == "public.statuses" {
				t.Error("lookup table (statuses) should be excluded")
			}
		}
	})

	t.Run("hub tables rank highest", func(t *testing.T) {
		if len(candidates) == 0 {
			t.Fatal("expected candidates")
		}
		// organizations or users should be top since they have the most children
		top := candidates[0]
		if top.Table != "public.organizations" && top.Table != "public.users" {
			t.Errorf("expected organizations or users at top, got %s", top.Table)
		}
	})

	t.Run("scores are descending", func(t *testing.T) {
		for i := 1; i < len(candidates); i++ {
			if candidates[i].Score > candidates[i-1].Score {
				t.Errorf("candidates not sorted: %s (%d) > %s (%d)",
					candidates[i].Table, candidates[i].Score,
					candidates[i-1].Table, candidates[i-1].Score)
			}
		}
	})

	t.Run("outgoing FKs penalize score", func(t *testing.T) {
		// projects has 2 outgoing FKs, organizations has 0
		var orgsScore, projScore int
		for _, c := range candidates {
			if c.Table == "public.organizations" {
				orgsScore = c.Score
			}
			if c.Table == "public.projects" {
				projScore = c.Score
			}
		}
		if projScore >= orgsScore {
			t.Errorf("projects (score %d) should rank below organizations (score %d) due to outgoing FK penalty",
				projScore, orgsScore)
		}
	})
}
