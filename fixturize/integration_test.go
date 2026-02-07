//go:build integration

package fixturize

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var testDB *sql.DB

const schemaDDL = `
CREATE TABLE organizations (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    secret_code TEXT
);

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    email TEXT NOT NULL,
    full_name TEXT
);

CREATE TABLE projects (
    id SERIAL PRIMARY KEY,
    owner_id INT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL
);

CREATE TABLE categories (
    id SERIAL PRIMARY KEY,
    parent_id INT REFERENCES categories(id),
    name TEXT NOT NULL
);

CREATE TABLE departments (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    lead_employee_id INT
);

CREATE TABLE employees (
    id SERIAL PRIMARY KEY,
    department_id INT NOT NULL REFERENCES departments(id) DEFERRABLE,
    name TEXT NOT NULL
);

ALTER TABLE departments
    ADD CONSTRAINT fk_dept_lead
    FOREIGN KEY (lead_employee_id) REFERENCES employees(id)
    DEFERRABLE;

CREATE TABLE audit_logs (
    id INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    action TEXT NOT NULL
);
`

func TestMain(m *testing.M) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:18",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
	}

	pg, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	host, _ := pg.Host(ctx)
	port, _ := pg.MappedPort(ctx, "5432")

	dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	testDB, err = sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open db: %v\n", err)
		os.Exit(1)
	}

	if _, err := testDB.Exec(schemaDDL); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create schema: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testDB.Close()
	pg.Terminate(ctx)
	os.Exit(code)
}

// --- helpers ---

var allTables = []string{
	"audit_logs", "categories", "departments", "employees",
	"organizations", "projects", "users",
}

func resetDB(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, table := range allTables {
		if _, err := db.Exec(fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table)); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}

func seedAll(t *testing.T, db *sql.DB) {
	t.Helper()
	queries := []string{
		`INSERT INTO organizations (id, name, secret_code) VALUES (1, 'Acme Corp', 'TOP_SECRET_123'), (2, 'Beta Inc', 'CLASSIFIED_456')`,
		`SELECT setval('organizations_id_seq', 2)`,
		`INSERT INTO users (id, org_id, email, full_name) VALUES (1, 1, 'alice@acme.com', 'Alice Smith'), (2, 1, 'bob@acme.com', 'Bob Jones'), (3, 2, 'carol@beta.com', 'Carol White')`,
		`SELECT setval('users_id_seq', 3)`,
		`INSERT INTO projects (id, owner_id, name) VALUES (1, 1, 'Project Alpha'), (2, 2, 'Project Beta')`,
		`SELECT setval('projects_id_seq', 2)`,
		`INSERT INTO categories (id, parent_id, name) VALUES (1, NULL, 'Root'), (2, 1, 'Child'), (3, 2, 'Grandchild')`,
		`SELECT setval('categories_id_seq', 3)`,
		`INSERT INTO departments (id, name, lead_employee_id) VALUES (1, 'Engineering', NULL)`,
		`INSERT INTO employees (id, department_id, name) VALUES (1, 1, 'Jane Lead')`,
		`UPDATE departments SET lead_employee_id = 1 WHERE id = 1`,
		`SELECT setval('departments_id_seq', 1)`,
		`SELECT setval('employees_id_seq', 1)`,
		`INSERT INTO audit_logs (org_id, action) VALUES (1, 'created_workspace')`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed query failed: %s\n  %v", q, err)
		}
	}
}

func seedCategories(t *testing.T, db *sql.DB) {
	t.Helper()
	queries := []string{
		`INSERT INTO categories (id, parent_id, name) VALUES (1, NULL, 'Root'), (2, 1, 'Child'), (3, 2, 'Grandchild')`,
		`SELECT setval('categories_id_seq', 3)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed categories failed: %s\n  %v", q, err)
		}
	}
}

func seedCircular(t *testing.T, db *sql.DB) {
	t.Helper()
	queries := []string{
		`INSERT INTO departments (id, name, lead_employee_id) VALUES (1, 'Engineering', NULL)`,
		`INSERT INTO employees (id, department_id, name) VALUES (1, 1, 'Jane Lead')`,
		`UPDATE departments SET lead_employee_id = 1 WHERE id = 1`,
		`SELECT setval('departments_id_seq', 1)`,
		`SELECT setval('employees_id_seq', 1)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("seed circular failed: %s\n  %v", q, err)
		}
	}
}

func extractFixture(t *testing.T, db *sql.DB, opts *ExtractOptions) *ExtractResult {
	t.Helper()
	if opts.Schema == "" {
		opts.Schema = "public"
	}
	e := NewExtractor(db, opts)
	result, err := e.Extract()
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	return result
}

func applyFixture(t *testing.T, db *sql.DB, fixture *Fixture, opts *ApplyOptions) *ApplyResult {
	t.Helper()
	a := NewApplier(db, opts)
	result, err := a.Apply(fixture)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	return result
}

func rowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func colIndex(ft *FixtureTable, name string) int {
	for i, c := range ft.Columns {
		if c == name {
			return i
		}
	}
	return -1
}

// --- tests ---

func TestTopologicalSort(t *testing.T) {
	schema, err := IntrospectSchema(testDB)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	tables := []string{"public.projects", "public.users", "public.organizations"}
	cycleCalled := false
	result := schema.TopologicalSort(tables, func(table string) {
		cycleCalled = true
	})

	if cycleCalled {
		t.Error("onCycle should not have been called for a DAG")
	}

	posOf := make(map[string]int)
	for i, t := range result {
		posOf[t] = i
	}

	if posOf["public.organizations"] >= posOf["public.users"] {
		t.Errorf("organizations (pos %d) should come before users (pos %d)", posOf["public.organizations"], posOf["public.users"])
	}
	if posOf["public.users"] >= posOf["public.projects"] {
		t.Errorf("users (pos %d) should come before projects (pos %d)", posOf["public.users"], posOf["public.projects"])
	}
}

func TestTopologicalSortCircular(t *testing.T) {
	schema, err := IntrospectSchema(testDB)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	tables := []string{"public.departments", "public.employees"}
	var cycledTables []string
	result := schema.TopologicalSort(tables, func(table string) {
		cycledTables = append(cycledTables, table)
	})

	if len(cycledTables) == 0 {
		t.Error("onCycle should have been called for circular FK")
	}

	found := make(map[string]bool)
	for _, r := range result {
		found[r] = true
	}
	if !found["public.departments"] || !found["public.employees"] {
		t.Errorf("both tables should be in result, got: %v", result)
	}
}

func TestFKTraversal(t *testing.T) {
	resetDB(t, testDB)
	seedAll(t, testDB)

	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "organizations WHERE id = 1",
		Schema: "public",
	})
	fixture := result.Fixture

	// organizations: exactly 1 row (id=1)
	orgs := fixture.Tables["public.organizations"]
	if orgs == nil {
		t.Fatal("missing public.organizations in fixture")
	}
	if len(orgs.Rows) != 1 {
		t.Errorf("expected 1 org row, got %d", len(orgs.Rows))
	}

	// users: 2 rows (Alice & Bob from org 1)
	users := fixture.Tables["public.users"]
	if users == nil {
		t.Fatal("missing public.users in fixture")
	}
	if len(users.Rows) != 2 {
		t.Errorf("expected 2 user rows, got %d", len(users.Rows))
	}

	// projects: 2 rows
	projects := fixture.Tables["public.projects"]
	if projects == nil {
		t.Fatal("missing public.projects in fixture")
	}
	if len(projects.Rows) != 2 {
		t.Errorf("expected 2 project rows, got %d", len(projects.Rows))
	}

	// org id=2 data should NOT be present
	orgIDIdx := colIndex(orgs, "id")
	if orgIDIdx < 0 {
		t.Fatal("no id column in organizations")
	}
	for _, row := range orgs.Rows {
		id := fmt.Sprintf("%v", row[orgIDIdx])
		if id == "2" {
			t.Error("org id=2 should not be in fixture")
		}
	}

	// Carol (org 2) should not appear
	if users != nil {
		emailIdx := colIndex(users, "email")
		if emailIdx >= 0 {
			for _, row := range users.Rows {
				if fmt.Sprintf("%v", row[emailIdx]) == "carol@beta.com" {
					t.Error("carol@beta.com (org 2) should not be in fixture")
				}
			}
		}
	}
}

func TestMaskApplication(t *testing.T) {
	resetDB(t, testDB)
	seedAll(t, testDB)

	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "organizations WHERE id = 1",
		Schema: "public",
		Mask:   []string{"organizations.secret_code='REDACTED'"},
	})
	fixture := result.Fixture

	orgs := fixture.Tables["public.organizations"]
	if orgs == nil {
		t.Fatal("missing public.organizations in fixture")
	}

	scIdx := colIndex(orgs, "secret_code")
	if scIdx < 0 {
		t.Fatal("no secret_code column in organizations")
	}

	for _, row := range orgs.Rows {
		val := fmt.Sprintf("%v", row[scIdx])
		if val != "REDACTED" {
			t.Errorf("expected secret_code='REDACTED', got %q", val)
		}
	}

	nameIdx := colIndex(orgs, "name")
	if nameIdx >= 0 {
		for _, row := range orgs.Rows {
			val := fmt.Sprintf("%v", row[nameIdx])
			if val != "Acme Corp" {
				t.Errorf("expected name='Acme Corp', got %q", val)
			}
		}
	}

	// Verify masks_applied in meta
	found := false
	for _, m := range fixture.Meta.MasksApplied {
		if m == "organizations.secret_code='REDACTED'" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("MasksApplied should contain the mask spec, got: %v", fixture.Meta.MasksApplied)
	}
}

func TestIdentityColumns(t *testing.T) {
	resetDB(t, testDB)
	seedAll(t, testDB)

	// get the source audit_logs id for comparison
	var sourceID int
	if err := testDB.QueryRow("SELECT id FROM audit_logs LIMIT 1").Scan(&sourceID); err != nil {
		t.Fatalf("query source audit_logs id: %v", err)
	}

	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "organizations WHERE id = 1",
		Schema: "public",
	})
	fixture := result.Fixture

	logs := fixture.Tables["public.audit_logs"]
	if logs == nil {
		t.Fatal("missing public.audit_logs in fixture")
	}

	idIdx := colIndex(logs, "id")
	if idIdx < 0 {
		t.Fatal("id column missing from audit_logs — identity column values not extracted")
	}

	// Verify extracted id matches source
	if len(logs.Rows) != 1 {
		t.Fatalf("expected 1 audit_logs row, got %d", len(logs.Rows))
	}
	extractedID := fmt.Sprintf("%v", logs.Rows[0][idIdx])
	if extractedID != fmt.Sprintf("%v", sourceID) {
		t.Errorf("expected audit_logs id=%d, got %s", sourceID, extractedID)
	}

	// Round-trip: truncate and re-apply
	resetDB(t, testDB)
	applyFixture(t, testDB, fixture, &ApplyOptions{Force: true})

	// Verify applied row matches
	var (
		appliedID int
		orgID     int
		action    string
	)

	err := testDB.QueryRow("SELECT id, org_id, action FROM audit_logs").Scan(&appliedID, &orgID, &action)
	if err != nil {
		t.Fatalf("query applied audit_logs: %v", err)
	}
	if appliedID != sourceID {
		t.Errorf("applied id=%d, want %d", appliedID, sourceID)
	}
	if orgID != 1 {
		t.Errorf("applied org_id=%d, want 1", orgID)
	}
	if action != "created_workspace" {
		t.Errorf("applied action=%q, want 'created_workspace'", action)
	}
}

func TestSelfReferencingFK(t *testing.T) {
	resetDB(t, testDB)
	seedCategories(t, testDB)

	// Extract starting from root category — should follow self-referencing FK
	// to discover the full hierarchy: Root → Child → Grandchild
	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "categories WHERE id = 1",
		Schema: "public",
	})
	fixture := result.Fixture

	cats := fixture.Tables["public.categories"]
	if cats == nil {
		t.Fatal("missing public.categories in fixture")
	}
	if len(cats.Rows) != 3 {
		t.Fatalf("expected 3 category rows (Root+Child+Grandchild), got %d", len(cats.Rows))
	}

	// Verify all three names are present
	nameIdx := colIndex(cats, "name")
	if nameIdx < 0 {
		t.Fatal("no name column in categories")
	}
	names := make(map[string]bool)
	for _, row := range cats.Rows {
		names[fmt.Sprintf("%v", row[nameIdx])] = true
	}
	for _, expected := range []string{"Root", "Child", "Grandchild"} {
		if !names[expected] {
			t.Errorf("missing category %q in fixture, got: %v", expected, names)
		}
	}

	// Round-trip: apply to empty DB and verify hierarchy is preserved
	resetDB(t, testDB)
	applyFixture(t, testDB, fixture, &ApplyOptions{Force: true, SyncSequences: true})

	if c := rowCount(t, testDB, "categories"); c != 3 {
		t.Fatalf("expected 3 categories after apply, got %d", c)
	}

	// Verify parent_id relationships
	var parentID *int
	err := testDB.QueryRow("SELECT parent_id FROM categories WHERE name = 'Root'").Scan(&parentID)
	if err != nil {
		t.Fatalf("query Root category: %v", err)
	}
	if parentID != nil {
		t.Errorf("Root parent_id should be NULL, got %v", *parentID)
	}

	var childParent int
	err = testDB.QueryRow("SELECT parent_id FROM categories WHERE name = 'Child'").Scan(&childParent)
	if err != nil {
		t.Fatalf("query Child category: %v", err)
	}

	var rootID int
	if err := testDB.QueryRow("SELECT id FROM categories WHERE name = 'Root'").Scan(&rootID); err != nil {
		t.Fatalf("query Root id: %v", err)
	}
	if childParent != rootID {
		t.Errorf("Child.parent_id=%d, want Root.id=%d", childParent, rootID)
	}
}

func TestCircularFKRoundTrip(t *testing.T) {
	resetDB(t, testDB)
	seedCircular(t, testDB)

	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "departments WHERE id = 1",
		Schema: "public",
	})
	fixture := result.Fixture

	depts := fixture.Tables["public.departments"]
	if depts == nil {
		t.Fatal("missing public.departments in fixture")
	}
	if len(depts.Rows) != 1 {
		t.Errorf("expected 1 department row, got %d", len(depts.Rows))
	}

	emps := fixture.Tables["public.employees"]
	if emps == nil {
		t.Fatal("missing public.employees in fixture")
	}
	if len(emps.Rows) != 1 {
		t.Errorf("expected 1 employee row, got %d", len(emps.Rows))
	}

	// Verify the circular references are captured
	leadIdx := colIndex(depts, "lead_employee_id")
	if leadIdx < 0 {
		t.Fatal("no lead_employee_id column in departments")
	}
	leadVal := fmt.Sprintf("%v", depts.Rows[0][leadIdx])
	if leadVal == "<nil>" {
		t.Error("department lead_employee_id should not be NULL")
	}

	deptIdx := colIndex(emps, "department_id")
	if deptIdx < 0 {
		t.Fatal("no department_id column in employees")
	}
	deptVal := fmt.Sprintf("%v", emps.Rows[0][deptIdx])
	if deptVal == "<nil>" {
		t.Error("employee department_id should not be NULL")
	}

	// Round-trip: apply with DEFERRABLE constraints
	resetDB(t, testDB)
	applyFixture(t, testDB, fixture, &ApplyOptions{Force: true, SyncSequences: true})

	if c := rowCount(t, testDB, "departments"); c != 1 {
		t.Fatalf("expected 1 department after apply, got %d", c)
	}
	if c := rowCount(t, testDB, "employees"); c != 1 {
		t.Fatalf("expected 1 employee after apply, got %d", c)
	}

	// Verify circular references survived the round-trip
	var appliedLead, appliedDept int
	err := testDB.QueryRow("SELECT lead_employee_id FROM departments WHERE id = 1").Scan(&appliedLead)
	if err != nil {
		t.Fatalf("query applied department lead: %v", err)
	}
	err = testDB.QueryRow("SELECT department_id FROM employees WHERE id = 1").Scan(&appliedDept)
	if err != nil {
		t.Fatalf("query applied employee dept: %v", err)
	}
	if appliedLead != 1 {
		t.Errorf("department lead_employee_id=%d, want 1", appliedLead)
	}
	if appliedDept != 1 {
		t.Errorf("employee department_id=%d, want 1", appliedDept)
	}
}

func TestFullRoundTrip(t *testing.T) {
	resetDB(t, testDB)
	seedAll(t, testDB)

	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "organizations WHERE id = 1",
		Schema: "public",
	})
	fixture := result.Fixture

	// Verify extracted tables and row counts
	expectedCounts := map[string]int{
		"public.organizations": 1,
		"public.users":         2,
		"public.projects":      2,
		"public.audit_logs":    1,
	}
	for table, wantCount := range expectedCounts {
		ft := fixture.Tables[table]
		if ft == nil {
			t.Errorf("missing %s in fixture", table)
			continue
		}
		if len(ft.Rows) != wantCount {
			t.Errorf("%s: got %d rows, want %d", table, len(ft.Rows), wantCount)
		}
	}

	// Serialize to JSON and deserialize to test the full pipeline
	jsonData, err := fixture.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	reloaded, err := LoadFixture(jsonData)
	if err != nil {
		t.Fatalf("LoadFixture: %v", err)
	}

	// Apply the reloaded fixture to a clean database
	resetDB(t, testDB)
	applyFixture(t, testDB, reloaded, &ApplyOptions{Force: true, SyncSequences: true})

	// Verify row counts match
	for table, wantCount := range expectedCounts {
		_, shortTable := parseTableName(table)
		if c := rowCount(t, testDB, shortTable); c != wantCount {
			t.Errorf("%s: got %d rows after apply, want %d", table, c, wantCount)
		}
	}

	// Verify FK integrity — all user.org_id values exist in organizations
	var badFKCount int
	err = testDB.QueryRow(`
		SELECT COUNT(*) FROM users u
		LEFT JOIN organizations o ON o.id = u.org_id
		WHERE o.id IS NULL
	`).Scan(&badFKCount)
	if err != nil {
		t.Fatalf("FK integrity check: %v", err)
	}
	if badFKCount > 0 {
		t.Errorf("found %d users with dangling org_id FK", badFKCount)
	}

	// Verify specific data values survived the round-trip
	var orgName string
	if err := testDB.QueryRow("SELECT name FROM organizations WHERE id = 1").Scan(&orgName); err != nil {
		t.Fatalf("query org name: %v", err)
	}
	if orgName != "Acme Corp" {
		t.Errorf("org name=%q, want 'Acme Corp'", orgName)
	}

	var userEmail string
	if err := testDB.QueryRow("SELECT email FROM users WHERE id = 1").Scan(&userEmail); err != nil {
		t.Fatalf("query user email: %v", err)
	}
	if userEmail != "alice@acme.com" {
		t.Errorf("user email=%q, want 'alice@acme.com'", userEmail)
	}

	// Verify org 2 data was NOT applied
	if c := rowCount(t, testDB, "organizations"); c != 1 {
		t.Errorf("expected exactly 1 organization, got %d", c)
	}

	// Verify sequences were synced — next insert should not conflict
	var newID int
	err = testDB.QueryRow("INSERT INTO users (org_id, email) VALUES (1, 'new@acme.com') RETURNING id").Scan(&newID)
	if err != nil {
		t.Fatalf("insert after sequence sync failed: %v", err)
	}
	if newID <= 2 {
		t.Errorf("new user id=%d, expected > 2 (sequence should be synced past existing ids)", newID)
	}
}
