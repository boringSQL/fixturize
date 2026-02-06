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
