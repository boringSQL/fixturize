package fixturize

import (
	"strings"
	"testing"
)

// TestFKColumnBase verifies that the fkColumnBase function correctly extracts
// the base name from foreign-key-style column names and properly rejects any
// column names that are not valid foreign key candidates.
func TestFKColumnBase(t *testing.T) {
	// Define a table of test cases that cover the various input scenarios.
	cases := []struct {
		name     string
		wantBase string
		wantOK   bool
	}{
		{"customer_id", "customer", true},
		{"Account_ID", "account", true}, // Verify that the matching is case-insensitive.
		{"id", "", false},               // A bare "id" is the table's own primary key.
		{"name", "", false},             // A column with no "_id" suffix is not a foreign key.
		{"_id", "", false},              // An empty base name must be rejected.
	}
	// Iterate over each of the test cases defined above.
	for _, c := range cases {
		// Call the function under test with the current case's input.
		base, ok := fkColumnBase(c.name)
		// Assert that both the returned base name and the OK flag match the
		// expected values for this test case.
		if ok != c.wantOK || base != c.wantBase {
			t.Errorf("fkColumnBase(%q) = (%q, %v), want (%q, %v)", c.name, base, ok, c.wantBase, c.wantOK)
		}
	}
}

// TestParentNameForms verifies that the parentNameForms function generates the
// correct pluralized table name forms for a given singular base name.
func TestParentNameForms(t *testing.T) {
	// Define a map of test cases where the key is the singular base name and
	// the value is the expected pluralized form.
	cases := map[string]string{
		"customer": "customers", // The simple case: just append an "s".
		"company":  "companies", // The "y" should be converted to "ies".
		"address":  "addresses", // A word ending in "s" should get an "es".
	}
	// Iterate over each base name and its expected plural form.
	for base, want := range cases {
		// Generate all of the candidate parent name forms.
		forms := parentNameForms(base)
		// Track whether we managed to find the expected form in the results.
		found := false
		// Loop through every generated form looking for a match.
		for _, f := range forms {
			if f == want {
				found = true
			}
		}
		// If the expected form was not found, the test fails.
		if !found {
			t.Errorf("parentNameForms(%q) = %v, missing %q", base, forms, want)
		}
	}
}

// TestTypesCompatible verifies that the typesCompatible function correctly
// determines whether two PostgreSQL column types are compatible enough to
// participate in a foreign key relationship.
func TestTypesCompatible(t *testing.T) {
	// Check that types within the same integer family are compatible.
	if !typesCompatible("integer", "bigint") {
		t.Error("integer family should be compatible")
	}
	// Check that varchar columns of differing lengths are still compatible.
	if !typesCompatible("character varying(255)", "character varying(100)") {
		t.Error("varchar of different lengths should be compatible")
	}
	// Check that an integer and a uuid are correctly reported as incompatible.
	if typesCompatible("integer", "uuid") {
		t.Error("integer and uuid must not be compatible")
	}
}

// TestSuggestMissingFKs verifies that the SuggestMissingFKs function correctly
// identifies columns that look like foreign keys but do not yet have a
// constraint declared.
func TestSuggestMissingFKs(t *testing.T) {
	// Arrange: build an in-memory schema with two tables to test against.
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			// The customers table acts as the parent table.
			"public.customers": {
				Schema: "public", Name: "customers",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id": {Name: "id", Type: "integer", IsPrimaryKey: true},
				},
			},
			// The orders table acts as the child table containing the columns
			// that we want to evaluate as foreign key candidates.
			"public.orders": {
				Schema: "public", Name: "orders",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":          {Name: "id", Type: "integer", OrdinalPosition: 1, IsPrimaryKey: true},
					"customer_id": {Name: "customer_id", Type: "integer", OrdinalPosition: 2},
					"region_id":   {Name: "region_id", Type: "integer", OrdinalPosition: 3, IsForeignKey: true},
					"widget_id":   {Name: "widget_id", Type: "integer", OrdinalPosition: 4},
				},
			},
		},
	}

	// Act: call the function under test to get the list of suggestions.
	got := SuggestMissingFKs(schema, []string{"public.customers", "public.orders"})

	// Assert: scan the returned suggestions and pull out the ones we care about.
	var customer, widget *FKSuggestion
	// Loop over every suggestion that was returned.
	for i := range got {
		// Inspect the child column to decide how to handle each suggestion.
		switch got[i].ChildColumn {
		case "customer_id":
			// Capture the customer_id suggestion for later assertions.
			customer = &got[i]
		case "widget_id":
			// Capture the widget_id suggestion for later assertions.
			widget = &got[i]
		case "region_id":
			// The region_id column already has a foreign key, so it should
			// never appear in the suggestions.
			t.Error("region_id already has an FK and must not be suggested")
		case "id":
			// A lone primary key column should never be suggested.
			t.Error("a lone primary key must not be suggested")
		}
	}

	// Verify that the customer_id column was suggested as expected.
	if customer == nil {
		t.Fatal("customer_id was not suggested")
	}
	// Verify that the customer_id suggestion points at the correct parent.
	if customer.ParentTable != "public.customers" || customer.ParentColumn != "id" {
		t.Errorf("customer_id should point at public.customers.id, got %s.%s", customer.ParentTable, customer.ParentColumn)
	}

	// Verify that the widget_id column was suggested as expected.
	if widget == nil {
		t.Fatal("widget_id was not suggested")
	}
	// Since there is no "widgets" table, the widget_id suggestion should not
	// have an associated parent table.
	if widget.ParentTable != "" {
		t.Errorf("widget_id has no parent table, got %q", widget.ParentTable)
	}
}

// TestFormatSuggestFKs verifies that the FormatSuggestFKs function produces a
// correctly formatted human-readable report for a given set of suggestions.
func TestFormatSuggestFKs(t *testing.T) {
	// Arrange: prepare a slice of suggestions covering the safe, orphaned, and
	// unmatched scenarios.
	suggestions := []FKSuggestion{
		{ChildTable: "public.orders", ChildColumn: "customer_id", ParentTable: "public.customers", ParentColumn: "id", Checked: true, OrphanCount: 0},
		{ChildTable: "public.events", ChildColumn: "account_id", ParentTable: "public.accounts", ParentColumn: "id", Checked: true, OrphanCount: 12},
		{ChildTable: "public.logs", ChildColumn: "ref_id"},
	}

	// Act: generate the report without showing unmatched columns.
	out := FormatSuggestFKs(suggestions, false)
	// Assert that the safe candidate is rendered correctly.
	if !strings.Contains(out, "✓ would validate") {
		t.Errorf("expected a safe candidate:\n%s", out)
	}
	// Assert that the orphaned candidate is rendered correctly.
	if !strings.Contains(out, "✗ 12 row(s) have no parent") {
		t.Errorf("expected an orphaned candidate:\n%s", out)
	}
	// Assert that the unmatched column is hidden when not explicitly requested.
	if strings.Contains(out, "ref_id") {
		t.Errorf("unmatched column should be hidden by default:\n%s", out)
	}
	// Assert that the summary line is present and accurate.
	if !strings.Contains(out, "2 candidate FK(s): 1 safe to add, 1 with orphaned data") {
		t.Errorf("expected summary line:\n%s", out)
	}

	// Act again: this time generate the report with unmatched columns shown.
	withUnmatched := FormatSuggestFKs(suggestions, true)
	// Assert that the unmatched column now appears in the output.
	if !strings.Contains(withUnmatched, "ref_id") {
		t.Errorf("expected unmatched column with --show-unmatched:\n%s", withUnmatched)
	}
}

// TestFormatSuggestFKsSQL verifies that the FormatSuggestFKsSQL function emits
// valid ALTER TABLE statements and correctly marks orphaned candidates as
// NOT VALID.
func TestFormatSuggestFKsSQL(t *testing.T) {
	// Arrange: prepare the same set of suggestions used in the report test.
	suggestions := []FKSuggestion{
		{ChildTable: "public.orders", ChildColumn: "customer_id", ParentTable: "public.customers", ParentColumn: "id", Checked: true, OrphanCount: 0},
		{ChildTable: "public.events", ChildColumn: "account_id", ParentTable: "public.accounts", ParentColumn: "id", Checked: true, OrphanCount: 12},
		{ChildTable: "public.logs", ChildColumn: "ref_id"},
	}

	// Act: generate the SQL output from the suggestions.
	sql := FormatSuggestFKsSQL(suggestions)
	// Assert that the constraint is given a properly derived name.
	if !strings.Contains(sql, `ADD CONSTRAINT "orders_customer_id_fkey"`) {
		t.Errorf("expected named constraint:\n%s", sql)
	}
	// Assert that the orphaned candidate produces a NOT VALID clause.
	if !strings.Contains(sql, "NOT VALID;") {
		t.Errorf("orphaned candidate should produce NOT VALID:\n%s", sql)
	}
	// Assert that exactly one statement is marked NOT VALID, since only the
	// orphaned candidate should receive that treatment.
	if strings.Count(sql, "NOT VALID;") != 1 {
		t.Errorf("only the orphaned candidate should be NOT VALID:\n%s", sql)
	}
}
