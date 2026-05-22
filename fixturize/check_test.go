package fixturize

import (
	"strings"
	"testing"
)

// TestCollectFKConstraints verifies that the collectFKConstraints function
// correctly groups foreign key columns by their constraint name and returns
// the resulting constraints in a deterministic, sorted order.
func TestCollectFKConstraints(t *testing.T) {
	// Arrange: build an in-memory schema containing two tables, one with a
	// simple single-column foreign key and one with a composite foreign key.
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			// The projects table acts as a table with a single-column FK.
			"public.projects": {
				Schema: "public", Name: "projects",
				ForeignKeys: []*ForeignKeyInfo{
					{ConstraintName: "projects_owner_id_fkey", ColumnName: "owner_id", ReferencedTable: "public.users", ReferencedColumn: "id"},
				},
			},
			// The item_notes table acts as a table with a composite FK, which
			// arrives from introspection as two rows sharing a constraint name.
			"public.item_notes": {
				Schema: "public", Name: "item_notes",
				ForeignKeys: []*ForeignKeyInfo{
					{ConstraintName: "item_notes_org_item_fkey", ColumnName: "org_id", ReferencedTable: "public.items", ReferencedColumn: "org_id"},
					{ConstraintName: "item_notes_org_item_fkey", ColumnName: "item_id", ReferencedTable: "public.items", ReferencedColumn: "id"},
				},
			},
		},
	}

	// Act: call the function under test to collect the FK constraints.
	got := collectFKConstraints(schema, []string{"public.projects", "public.item_notes"})

	// Assert: we expect exactly two constraints to have been collected.
	if len(got) != 2 {
		t.Fatalf("expected 2 constraints, got %d", len(got))
	}

	// Assert that the results are sorted by child table, so item_notes comes
	// before projects.
	if got[0].ChildTable != "public.item_notes" {
		t.Errorf("expected item_notes first, got %s", got[0].ChildTable)
	}
	// Assert that the composite FK columns were grouped together in order.
	if len(got[0].ChildCols) != 2 || got[0].ChildCols[0] != "org_id" || got[0].ChildCols[1] != "item_id" {
		t.Errorf("composite FK columns not grouped: %v", got[0].ChildCols)
	}
	// Assert that the single-column FK produced exactly one column.
	if len(got[1].ChildCols) != 1 || got[1].ChildCols[0] != "owner_id" {
		t.Errorf("single FK columns wrong: %v", got[1].ChildCols)
	}
}

// TestOrphanWhereSingleColumn verifies that the orphanWhere function builds the
// correct SQL predicate for a foreign key constraint that has a single column.
func TestOrphanWhereSingleColumn(t *testing.T) {
	// Arrange: define a single-column foreign key constraint to test against.
	c := FKConstraint{
		ChildTable:  "public.projects",
		ParentTable: "public.users",
		ChildCols:   []string{"owner_id"},
		ParentCols:  []string{"id"},
	}

	// Act: call the function under test to generate the WHERE predicate.
	got := orphanWhere(c)
	// Assert that the generated predicate exactly matches the expected SQL.
	want := `c."owner_id" IS NOT NULL AND NOT EXISTS (SELECT 1 FROM "public"."users" p WHERE p."id" = c."owner_id")`
	if got != want {
		t.Errorf("orphanWhere mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// TestOrphanWhereComposite verifies that the orphanWhere function builds the
// correct SQL predicate for a foreign key constraint that spans multiple
// columns, ensuring every column is tied together in the generated join.
func TestOrphanWhereComposite(t *testing.T) {
	// Arrange: define a composite foreign key constraint to test against.
	c := FKConstraint{
		ChildTable:  "public.item_notes",
		ParentTable: "public.items",
		ChildCols:   []string{"org_id", "item_id"},
		ParentCols:  []string{"org_id", "id"},
	}

	// Act: call the function under test to generate the WHERE predicate.
	got := orphanWhere(c)
	// Assert that the generated predicate exactly matches the expected SQL.
	want := `c."org_id" IS NOT NULL AND c."item_id" IS NOT NULL AND NOT EXISTS ` +
		`(SELECT 1 FROM "public"."items" p WHERE p."org_id" = c."org_id" AND p."id" = c."item_id")`
	if got != want {
		t.Errorf("orphanWhere mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// TestFormatCheckClean verifies that the FormatCheck function renders a clean,
// reassuring summary when none of the constraints have any orphaned rows.
func TestFormatCheckClean(t *testing.T) {
	// Arrange: prepare a single constraint result with a zero orphan count.
	results := []OrphanResult{
		{Constraint: FKConstraint{ConstraintName: "a_fkey", ChildTable: "public.a", ParentTable: "public.b", ChildCols: []string{"b_id"}, ParentCols: []string{"id"}}},
	}

	// Act: generate the human-readable report from the results.
	out := FormatCheck(results)
	// Assert that a check mark is present for the clean constraint.
	if !strings.Contains(out, "✓") {
		t.Errorf("expected check mark for clean constraint:\n%s", out)
	}
	// Assert that the summary line confirms all constraints are satisfied.
	if !strings.Contains(out, "All 1 FK constraint(s) satisfied") {
		t.Errorf("expected clean summary:\n%s", out)
	}
}

// TestFormatCheckWithOrphans verifies that the FormatCheck function correctly
// renders a failing constraint, its sample rows, and an accurate summary when
// orphaned rows are present.
func TestFormatCheckWithOrphans(t *testing.T) {
	// Arrange: prepare a constraint result that has five orphaned rows along
	// with two sample rows to display.
	results := []OrphanResult{
		{
			Constraint:    FKConstraint{ConstraintName: "p_owner_fkey", ChildTable: "public.projects", ParentTable: "public.users", ChildCols: []string{"owner_id"}, ParentCols: []string{"id"}},
			OrphanCount:   5,
			SampleColumns: []string{"id", "owner_id"},
			Samples: []map[string]any{
				{"id": int32(1), "owner_id": int32(9999)},
				{"id": int32(2), "owner_id": int32(9999)},
			},
		},
	}

	// Act: generate the human-readable report from the results.
	out := FormatCheck(results)
	// Assert that the failing constraint line is rendered.
	if !strings.Contains(out, "✗ p_owner_fkey") {
		t.Errorf("expected failing constraint line:\n%s", out)
	}
	// Assert that the FK arrow showing the relationship is rendered.
	if !strings.Contains(out, "owner_id → public.users.id") {
		t.Errorf("expected FK arrow:\n%s", out)
	}
	// Assert that one of the sample rows appears in the output.
	if !strings.Contains(out, "owner_id=9999") {
		t.Errorf("expected sample row:\n%s", out)
	}
	// Assert that the remaining-count line accounts for the rows beyond the
	// two that were shown as samples.
	if !strings.Contains(out, "... and 3 more") {
		t.Errorf("expected remaining-count line:\n%s", out)
	}
	// Assert that the summary line reports the orphan totals accurately.
	if !strings.Contains(out, "5 orphan row(s) in 1 of 1 constraint(s)") {
		t.Errorf("expected orphan summary:\n%s", out)
	}
}

// TestFormatCheckSQL verifies that the FormatCheckSQL function emits a DELETE
// statement only for those constraints that actually have orphaned rows.
func TestFormatCheckSQL(t *testing.T) {
	// Arrange: prepare two constraint results, one that is clean and one that
	// has orphaned rows requiring cleanup.
	results := []OrphanResult{
		{Constraint: FKConstraint{ConstraintName: "clean_fkey", ChildTable: "public.x", ParentTable: "public.y", ChildCols: []string{"y_id"}, ParentCols: []string{"id"}}},
		{
			Constraint:  FKConstraint{ConstraintName: "p_owner_fkey", ChildTable: "public.projects", ParentTable: "public.users", ChildCols: []string{"owner_id"}, ParentCols: []string{"id"}},
			OrphanCount: 5,
		},
	}

	// Act: generate the SQL cleanup output from the results.
	sql := FormatCheckSQL(results)
	// Assert that the orphaned constraint produced a DELETE statement.
	if !strings.Contains(sql, `DELETE FROM "public"."projects" AS c`) {
		t.Errorf("expected DELETE for orphaned constraint:\n%s", sql)
	}
	// Assert that the clean constraint did not produce a DELETE statement.
	if strings.Contains(sql, "public.x") {
		t.Errorf("clean constraint should not produce a DELETE:\n%s", sql)
	}
}

// TestTotalOrphans verifies that the TotalOrphans function correctly sums the
// orphan counts across every constraint result that it is given.
func TestTotalOrphans(t *testing.T) {
	// Arrange: prepare a set of results with a mix of orphan counts.
	results := []OrphanResult{
		{OrphanCount: 3},
		{OrphanCount: 0},
		{OrphanCount: 7},
	}
	// Act and Assert: the summed total should equal ten.
	if got := TotalOrphans(results); got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}
