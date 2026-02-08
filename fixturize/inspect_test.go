package fixturize

import (
	"strings"
	"testing"
)

func TestReachableSubgraph(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.organizations": {Schema: "public", Name: "organizations"},
			"public.users": {
				Schema: "public", Name: "users",
				ForeignKeys: []*ForeignKeyInfo{
					{ColumnName: "org_id", ReferencedTable: "public.organizations", ReferencedColumn: "id"},
				},
			},
			"public.posts": {
				Schema: "public", Name: "posts",
				ForeignKeys: []*ForeignKeyInfo{
					{ColumnName: "user_id", ReferencedTable: "public.users", ReferencedColumn: "id"},
				},
			},
			"public.tags": {Schema: "public", Name: "tags"},
		},
	}

	t.Run("all reachable unlimited", func(t *testing.T) {
		tables, err := ReachableSubgraph(schema, "public.users", 0)
		if err != nil {
			t.Fatal(err)
		}
		// users connects to organizations (parent) and posts (child), but not tags
		if len(tables) != 3 {
			t.Fatalf("expected 3 tables, got %d: %v", len(tables), tables)
		}
		for _, want := range []string{"public.organizations", "public.users", "public.posts"} {
			found := false
			for _, got := range tables {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %s in result", want)
			}
		}
	})

	t.Run("depth 1", func(t *testing.T) {
		tables, err := ReachableSubgraph(schema, "public.users", 1)
		if err != nil {
			t.Fatal(err)
		}
		// depth 1 from users: organizations and posts are direct neighbors
		if len(tables) != 3 {
			t.Fatalf("expected 3 tables, got %d: %v", len(tables), tables)
		}
	})

	t.Run("depth 1 from orgs", func(t *testing.T) {
		tables, err := ReachableSubgraph(schema, "public.organizations", 1)
		if err != nil {
			t.Fatal(err)
		}
		// depth 1 from organizations: only users is direct neighbor
		if len(tables) != 2 {
			t.Fatalf("expected 2 tables, got %d: %v", len(tables), tables)
		}
	})

	t.Run("unknown root", func(t *testing.T) {
		_, err := ReachableSubgraph(schema, "public.nonexistent", 0)
		if err == nil {
			t.Fatal("expected error for unknown root")
		}
	})

	t.Run("unqualified root", func(t *testing.T) {
		tables, err := ReachableSubgraph(schema, "users", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(tables) != 3 {
			t.Fatalf("expected 3 tables, got %d: %v", len(tables), tables)
		}
	})
}

func TestFormatInspect(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2, IsUnique: true},
					"org_id": {
						Name: "org_id", Type: "bigint", OrdinalPosition: 3,
						IsForeignKey: true,
						ForeignKey:   &ForeignKeyInfo{ReferencedTable: "public.organizations", ReferencedColumn: "id"},
						IsNullable:   true,
					},
				},
				ForeignKeys: []*ForeignKeyInfo{
					{ColumnName: "org_id", ReferencedTable: "public.organizations", ReferencedColumn: "id"},
				},
			},
		},
	}

	output := FormatInspect(schema, []string{"public.users"})

	if !strings.Contains(output, "public.users") {
		t.Error("expected table name in output")
	}
	if !strings.Contains(output, "PK") {
		t.Error("expected PK annotation")
	}
	if !strings.Contains(output, "character varying(255)") {
		t.Error("expected character varying(255) type")
	}
	if !strings.Contains(output, "UNIQUE") {
		t.Error("expected UNIQUE annotation")
	}
	if !strings.Contains(output, "FK → public.organizations.id") {
		t.Error("expected FK annotation")
	}
	if !strings.Contains(output, "NOT NULL") {
		t.Errorf("expected NOT NULL for non-nullable columns, got:\n%s", output)
	}
}

