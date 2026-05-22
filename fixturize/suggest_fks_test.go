package fixturize

import (
	"strings"
	"testing"
)

func TestFKColumnBase(t *testing.T) {
	cases := []struct {
		name     string
		wantBase string
		wantOK   bool
	}{
		{"customer_id", "customer", true},
		{"Account_ID", "account", true}, // case-insensitive
		{"id", "", false},               // bare id is the table's own PK
		{"name", "", false},             // no _id suffix
		{"_id", "", false},              // empty base
	}
	for _, c := range cases {
		base, ok := fkColumnBase(c.name)
		if ok != c.wantOK || base != c.wantBase {
			t.Errorf("fkColumnBase(%q) = (%q, %v), want (%q, %v)", c.name, base, ok, c.wantBase, c.wantOK)
		}
	}
}

func TestParentNameForms(t *testing.T) {
	cases := map[string]string{
		"customer": "customers", // +s
		"company":  "companies", // y -> ies
		"address":  "addresses", // +es
	}
	for base, want := range cases {
		forms := parentNameForms(base)
		found := false
		for _, f := range forms {
			if f == want {
				found = true
			}
		}
		if !found {
			t.Errorf("parentNameForms(%q) = %v, missing %q", base, forms, want)
		}
	}
}

func TestTypesCompatible(t *testing.T) {
	if !typesCompatible("integer", "bigint") {
		t.Error("integer family should be compatible")
	}
	if !typesCompatible("character varying(255)", "character varying(100)") {
		t.Error("varchar of different lengths should be compatible")
	}
	if typesCompatible("integer", "uuid") {
		t.Error("integer and uuid must not be compatible")
	}
}

func TestSuggestMissingFKs(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.customers": {
				Schema: "public", Name: "customers",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id": {Name: "id", Type: "integer", IsPrimaryKey: true},
				},
			},
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

	got := SuggestMissingFKs(schema, []string{"public.customers", "public.orders"})

	var customer, widget *FKSuggestion
	for i := range got {
		switch got[i].ChildColumn {
		case "customer_id":
			customer = &got[i]
		case "widget_id":
			widget = &got[i]
		case "region_id":
			t.Error("region_id already has an FK and must not be suggested")
		case "id":
			t.Error("a lone primary key must not be suggested")
		}
	}

	if customer == nil {
		t.Fatal("customer_id was not suggested")
	}
	if customer.ParentTable != "public.customers" || customer.ParentColumn != "id" {
		t.Errorf("customer_id should point at public.customers.id, got %s.%s", customer.ParentTable, customer.ParentColumn)
	}

	if widget == nil {
		t.Fatal("widget_id was not suggested")
	}
	// no widgets table, so no parent
	if widget.ParentTable != "" {
		t.Errorf("widget_id has no parent table, got %q", widget.ParentTable)
	}
}

func TestFormatSuggestFKs(t *testing.T) {
	suggestions := []FKSuggestion{
		{ChildTable: "public.orders", ChildColumn: "customer_id", ParentTable: "public.customers", ParentColumn: "id", Checked: true, OrphanCount: 0},
		{ChildTable: "public.events", ChildColumn: "account_id", ParentTable: "public.accounts", ParentColumn: "id", Checked: true, OrphanCount: 12},
		{ChildTable: "public.logs", ChildColumn: "ref_id"},
	}

	out := FormatSuggestFKs(suggestions, false)
	if !strings.Contains(out, "✓ would validate") {
		t.Errorf("expected a safe candidate:\n%s", out)
	}
	if !strings.Contains(out, "✗ 12 row(s) have no parent") {
		t.Errorf("expected an orphaned candidate:\n%s", out)
	}
	// unmatched columns hidden unless requested
	if strings.Contains(out, "ref_id") {
		t.Errorf("unmatched column should be hidden by default:\n%s", out)
	}
	if !strings.Contains(out, "2 candidate FK(s): 1 safe to add, 1 with orphaned data") {
		t.Errorf("expected summary line:\n%s", out)
	}

	withUnmatched := FormatSuggestFKs(suggestions, true)
	if !strings.Contains(withUnmatched, "ref_id") {
		t.Errorf("expected unmatched column with --show-unmatched:\n%s", withUnmatched)
	}
}

func TestFormatSuggestFKsSQL(t *testing.T) {
	suggestions := []FKSuggestion{
		{ChildTable: "public.orders", ChildColumn: "customer_id", ParentTable: "public.customers", ParentColumn: "id", Checked: true, OrphanCount: 0},
		{ChildTable: "public.events", ChildColumn: "account_id", ParentTable: "public.accounts", ParentColumn: "id", Checked: true, OrphanCount: 12},
		{ChildTable: "public.logs", ChildColumn: "ref_id"},
	}

	sql := FormatSuggestFKsSQL(suggestions)
	if !strings.Contains(sql, `ADD CONSTRAINT "orders_customer_id_fkey"`) {
		t.Errorf("expected named constraint:\n%s", sql)
	}
	// only the orphaned candidate gets NOT VALID
	if !strings.Contains(sql, "NOT VALID;") {
		t.Errorf("orphaned candidate should produce NOT VALID:\n%s", sql)
	}
	if strings.Count(sql, "NOT VALID;") != 1 {
		t.Errorf("only the orphaned candidate should be NOT VALID:\n%s", sql)
	}
}
