//go:build integration

package fixturize

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
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

CREATE TABLE events (
    id SERIAL,
    org_id INT NOT NULL REFERENCES organizations(id),
    event_date DATE NOT NULL,
    payload TEXT,
    PRIMARY KEY (id, event_date)
) PARTITION BY RANGE (event_date);

CREATE TABLE events_2024 PARTITION OF events
    FOR VALUES FROM ('2024-01-01') TO ('2025-01-01');
CREATE TABLE events_2025 PARTITION OF events
    FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');

CREATE TABLE tasks (
    id SERIAL PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    title TEXT NOT NULL
) PARTITION BY HASH (id);

CREATE TABLE tasks_hash_0 PARTITION OF tasks FOR VALUES WITH (MODULUS 4, REMAINDER 0);
CREATE TABLE tasks_hash_1 PARTITION OF tasks FOR VALUES WITH (MODULUS 4, REMAINDER 1);
CREATE TABLE tasks_hash_2 PARTITION OF tasks FOR VALUES WITH (MODULUS 4, REMAINDER 2);
CREATE TABLE tasks_hash_3 PARTITION OF tasks FOR VALUES WITH (MODULUS 4, REMAINDER 3);

CREATE TABLE task_comments (
    id SERIAL PRIMARY KEY,
    task_id INT NOT NULL REFERENCES tasks(id),
    body TEXT NOT NULL
);

CREATE TABLE metrics (
    id SERIAL,
    org_id INT NOT NULL REFERENCES organizations(id),
    recorded_at DATE NOT NULL,
    value INT NOT NULL,
    PRIMARY KEY (id, recorded_at)
) PARTITION BY RANGE (recorded_at);

CREATE TABLE metrics_2024 PARTITION OF metrics
    FOR VALUES FROM ('2024-01-01') TO ('2025-01-01')
    PARTITION BY HASH (id);
CREATE TABLE metrics_2024_hash_0 PARTITION OF metrics_2024 FOR VALUES WITH (MODULUS 2, REMAINDER 0);
CREATE TABLE metrics_2024_hash_1 PARTITION OF metrics_2024 FOR VALUES WITH (MODULUS 2, REMAINDER 1);

CREATE TABLE metrics_2025 PARTITION OF metrics
    FOR VALUES FROM ('2025-01-01') TO ('2026-01-01');

CREATE TABLE workspace_projects (
    workspace_id INT NOT NULL,
    project_id INT NOT NULL,
    name TEXT NOT NULL,
    PRIMARY KEY (workspace_id, project_id)
);

CREATE TABLE project_assignments (
    id SERIAL PRIMARY KEY,
    workspace_id INT NOT NULL,
    project_id INT NOT NULL,
    assignee TEXT NOT NULL,
    FOREIGN KEY (workspace_id, project_id) REFERENCES workspace_projects(workspace_id, project_id)
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
	"events", "metrics", "organizations", "project_assignments",
	"projects", "task_comments", "tasks", "users",
	"workspace_projects",
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
		`INSERT INTO events (id, org_id, event_date, payload) VALUES (1, 1, '2024-06-15', 'event_a'), (2, 1, '2025-03-20', 'event_b'), (3, 2, '2024-11-01', 'event_c')`,
		`SELECT setval('events_id_seq', 3)`,
		`INSERT INTO tasks (id, org_id, title) VALUES (1, 1, 'Task A'), (2, 1, 'Task B'), (3, 2, 'Task C')`,
		`SELECT setval('tasks_id_seq', 3)`,
		`INSERT INTO task_comments (id, task_id, body) VALUES (1, 1, 'comment on A'), (2, 2, 'comment on B'), (3, 3, 'comment on C')`,
		`SELECT setval('task_comments_id_seq', 3)`,
		`INSERT INTO metrics (id, org_id, recorded_at, value) VALUES (1, 1, '2024-03-15', 100), (2, 1, '2024-09-20', 200), (3, 1, '2025-02-10', 300), (4, 2, '2024-06-01', 400)`,
		`SELECT setval('metrics_id_seq', 4)`,
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
	result, err := a.Apply(context.Background(), fixture)
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

func TestPartitionedTable(t *testing.T) {
	resetDB(t, testDB)
	seedAll(t, testDB)

	schema, err := IntrospectSchema(testDB)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	// Range-partitioned: parent visible, partitions hidden
	if !schema.HasTable("public.events") {
		t.Fatal("partitioned parent table 'events' should be visible in schema")
	}
	if schema.HasTable("public.events_2024") {
		t.Error("partition 'events_2024' should NOT appear as a separate table")
	}
	if schema.HasTable("public.events_2025") {
		t.Error("partition 'events_2025' should NOT appear as a separate table")
	}

	// Hash-partitioned: parent visible, partitions hidden
	if !schema.HasTable("public.tasks") {
		t.Fatal("partitioned parent table 'tasks' should be visible in schema")
	}
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("public.tasks_hash_%d", i)
		if schema.HasTable(name) {
			t.Errorf("partition %q should NOT appear as a separate table", name)
		}
	}

	// FK on task_comments should reference 'tasks' (parent), not a partition
	tcInfo, err := schema.GetTable("public.task_comments")
	if err != nil {
		t.Fatal(err)
	}
	for _, fk := range tcInfo.ForeignKeys {
		if fk.ColumnName == "task_id" && fk.ReferencedTable != "public.tasks" {
			t.Errorf("task_comments.task_id FK references %q, want 'public.tasks'", fk.ReferencedTable)
		}
	}

	// Extract org 1 — should pull events and tasks across partitions
	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "organizations WHERE id = 1",
		Schema: "public",
	})
	fixture := result.Fixture

	// Range-partitioned events: 2 rows across 2 partitions
	events := fixture.Tables["public.events"]
	if events == nil {
		t.Fatal("missing public.events in fixture")
	}
	if len(events.Rows) != 2 {
		t.Errorf("expected 2 event rows for org 1, got %d", len(events.Rows))
	}

	payloadIdx := colIndex(events, "payload")
	if payloadIdx < 0 {
		t.Fatal("no payload column in events")
	}
	payloads := make(map[string]bool)
	for _, row := range events.Rows {
		payloads[fmt.Sprintf("%v", row[payloadIdx])] = true
	}
	if !payloads["event_a"] || !payloads["event_b"] {
		t.Errorf("expected event_a and event_b, got: %v", payloads)
	}
	if payloads["event_c"] {
		t.Error("event_c (org 2) should not be in fixture")
	}

	// Hash-partitioned tasks: 2 rows for org 1, spread across hash partitions
	tasks := fixture.Tables["public.tasks"]
	if tasks == nil {
		t.Fatal("missing public.tasks in fixture")
	}
	if len(tasks.Rows) != 2 {
		t.Errorf("expected 2 task rows for org 1, got %d", len(tasks.Rows))
	}

	// task_comments should be extracted as children of the parent table
	comments := fixture.Tables["public.task_comments"]
	if comments == nil {
		t.Fatal("missing public.task_comments in fixture")
	}
	if len(comments.Rows) != 2 {
		t.Errorf("expected 2 task_comments for org 1 tasks, got %d", len(comments.Rows))
	}

	// No partition tables should appear in fixture
	for tableName := range fixture.Tables {
		if strings.Contains(tableName, "hash_") || strings.Contains(tableName, "_2024") || strings.Contains(tableName, "_2025") {
			t.Errorf("partition %q should not appear in fixture", tableName)
		}
	}

	// Round-trip
	resetDB(t, testDB)
	applyFixture(t, testDB, fixture, &ApplyOptions{Force: true, SyncSequences: true})

	if c := rowCount(t, testDB, "events"); c != 2 {
		t.Errorf("expected 2 events after apply, got %d", c)
	}
	if c := rowCount(t, testDB, "tasks"); c != 2 {
		t.Errorf("expected 2 tasks after apply, got %d", c)
	}
	if c := rowCount(t, testDB, "task_comments"); c != 2 {
		t.Errorf("expected 2 task_comments after apply, got %d", c)
	}

	// Verify FK from task_comments -> tasks survived round-trip
	var badFK int
	err = testDB.QueryRow(`
		SELECT COUNT(*) FROM task_comments tc
		LEFT JOIN tasks t ON t.id = tc.task_id
		WHERE t.id IS NULL
	`).Scan(&badFK)
	if err != nil {
		t.Fatalf("FK check: %v", err)
	}
	if badFK > 0 {
		t.Errorf("found %d task_comments with dangling task_id FK", badFK)
	}
}

func TestNestedPartitions(t *testing.T) {
	resetDB(t, testDB)
	seedAll(t, testDB)

	schema, err := IntrospectSchema(testDB)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	// Only the root partitioned table should be visible
	if !schema.HasTable("public.metrics") {
		t.Fatal("root partitioned table 'metrics' should be visible")
	}
	for _, hidden := range []string{
		"public.metrics_2024", "public.metrics_2025",
		"public.metrics_2024_hash_0", "public.metrics_2024_hash_1",
	} {
		if schema.HasTable(hidden) {
			t.Errorf("partition %q should NOT appear as a separate table", hidden)
		}
	}

	// Verify partition map resolves nested partitions to root
	partMap, err := getPartitionParents(testDB)
	if err != nil {
		t.Fatalf("getPartitionParents: %v", err)
	}
	// Sub-partitions should map to the root, not the intermediate
	for _, leaf := range []string{"public.metrics_2024_hash_0", "public.metrics_2024_hash_1"} {
		if root, ok := partMap[leaf]; !ok {
			t.Errorf("partition map missing %q", leaf)
		} else if root != "public.metrics" {
			t.Errorf("partition %q maps to %q, want 'public.metrics'", leaf, root)
		}
	}
	// Intermediate partition should also map to root
	if root, ok := partMap["public.metrics_2024"]; !ok {
		t.Errorf("partition map missing 'public.metrics_2024'")
	} else if root != "public.metrics" {
		t.Errorf("metrics_2024 maps to %q, want 'public.metrics'", root)
	}

	// Extract org 1 — should get 3 metrics across nested partitions
	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "organizations WHERE id = 1",
		Schema: "public",
	})
	fixture := result.Fixture

	metrics := fixture.Tables["public.metrics"]
	if metrics == nil {
		t.Fatal("missing public.metrics in fixture")
	}
	if len(metrics.Rows) != 3 {
		t.Errorf("expected 3 metric rows for org 1, got %d", len(metrics.Rows))
	}

	// No nested partitions in fixture
	for tableName := range fixture.Tables {
		if strings.Contains(tableName, "metrics_") {
			t.Errorf("partition %q should not appear in fixture", tableName)
		}
	}

	// Round-trip
	resetDB(t, testDB)
	applyFixture(t, testDB, fixture, &ApplyOptions{Force: true, SyncSequences: true})

	if c := rowCount(t, testDB, "metrics"); c != 3 {
		t.Errorf("expected 3 metrics after apply, got %d", c)
	}
}

func TestAnalyzeSchemaIntegration(t *testing.T) {
	schema, err := IntrospectSchema(testDB)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	tables := schema.GetTables()
	result := AnalyzeSchema(schema, tables, ConfidenceLow)

	// users.email (text) should be detected as Email with HIGH confidence
	userMatches := result.Matches["public.users"]
	var emailMatch, fullNameMatch *PIIMatch
	for i, m := range userMatches {
		switch m.Column {
		case "email":
			emailMatch = &userMatches[i]
		case "full_name":
			fullNameMatch = &userMatches[i]
		}
	}

	if emailMatch == nil {
		t.Error("expected users.email to be detected as PII")
	} else {
		if emailMatch.Category != "Email" {
			t.Errorf("users.email category = %q, want Email", emailMatch.Category)
		}
		if emailMatch.Confidence < ConfidenceHigh {
			t.Errorf("users.email confidence = %d, want >= HIGH", emailMatch.Confidence)
		}
	}

	// users.full_name (text) should be detected as Full Name with HIGH confidence
	if fullNameMatch == nil {
		t.Error("expected users.full_name to be detected as PII")
	} else {
		if fullNameMatch.Category != "Full Name" {
			t.Errorf("users.full_name category = %q, want Full Name", fullNameMatch.Category)
		}
		if fullNameMatch.Confidence < ConfidenceHigh {
			t.Errorf("users.full_name confidence = %d, want >= HIGH", fullNameMatch.Confidence)
		}
	}

	// PK columns must never appear in matches
	for _, matches := range result.Matches {
		for _, m := range matches {
			if m.Column == "id" {
				t.Errorf("PK column 'id' should not be detected in %s", m.Table)
			}
		}
	}

	// FK columns must never appear in matches
	fkColumns := map[string]bool{
		"org_id": true, "owner_id": true, "parent_id": true,
		"department_id": true, "lead_employee_id": true, "task_id": true,
		"workspace_id": true, "project_id": true,
	}
	for _, matches := range result.Matches {
		for _, m := range matches {
			if fkColumns[m.Column] {
				t.Errorf("FK column %q should not be detected in %s", m.Column, m.Table)
			}
		}
	}

	// Bare 'name' columns should NOT match any PII rule
	for table, matches := range result.Matches {
		for _, m := range matches {
			if m.Column == "name" {
				t.Errorf("bare 'name' column should not be detected as PII in %s", table)
			}
		}
	}

	// Mask expressions should reference real PK columns
	if emailMatch != nil {
		if !strings.Contains(emailMatch.MaskExpr, `"id"`) {
			t.Errorf("email mask should reference PK column \"id\", got %q", emailMatch.MaskExpr)
		}
	}
}

func TestCompositeForeignKey(t *testing.T) {
	resetDB(t, testDB)

	// Seed workspace_projects and project_assignments
	queries := []string{
		`INSERT INTO workspace_projects (workspace_id, project_id, name) VALUES (10, 1, 'WP-A'), (10, 2, 'WP-B')`,
		`INSERT INTO project_assignments (id, workspace_id, project_id, assignee) VALUES (1, 10, 1, 'Alice'), (2, 10, 2, 'Bob')`,
		`SELECT setval('project_assignments_id_seq', 2)`,
	}
	for _, q := range queries {
		if _, err := testDB.Exec(q); err != nil {
			t.Fatalf("seed: %s\n  %v", q, err)
		}
	}

	schema, err := IntrospectSchema(testDB)
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	// Verify composite FK has correct column pairings (not a cross-product)
	paInfo, err := schema.GetTable("public.project_assignments")
	if err != nil {
		t.Fatal(err)
	}

	// Should have exactly 2 FK entries (one per column in the composite FK)
	if len(paInfo.ForeignKeys) != 2 {
		t.Fatalf("expected 2 FK entries for composite FK, got %d", len(paInfo.ForeignKeys))
	}

	// Build a map of local_col -> referenced_col for verification
	fkPairs := make(map[string]string)
	for _, fk := range paInfo.ForeignKeys {
		fkPairs[fk.ColumnName] = fk.ReferencedColumn
		if fk.ReferencedTable != "public.workspace_projects" {
			t.Errorf("FK references %q, want 'public.workspace_projects'", fk.ReferencedTable)
		}
	}

	// workspace_id -> workspace_id, project_id -> project_id (not cross-product)
	if fkPairs["workspace_id"] != "workspace_id" {
		t.Errorf("workspace_id FK maps to %q, want 'workspace_id'", fkPairs["workspace_id"])
	}
	if fkPairs["project_id"] != "project_id" {
		t.Errorf("project_id FK maps to %q, want 'project_id'", fkPairs["project_id"])
	}

	// Extract from workspace_projects — should pull assignments as children
	result := extractFixture(t, testDB, &ExtractOptions{
		Root:   "workspace_projects WHERE workspace_id = 10",
		Schema: "public",
	})
	fixture := result.Fixture

	wp := fixture.Tables["public.workspace_projects"]
	if wp == nil {
		t.Fatal("missing public.workspace_projects in fixture")
	}
	if len(wp.Rows) != 2 {
		t.Errorf("expected 2 workspace_projects rows, got %d", len(wp.Rows))
	}

	pa := fixture.Tables["public.project_assignments"]
	if pa == nil {
		t.Fatal("missing public.project_assignments in fixture")
	}
	if len(pa.Rows) != 2 {
		t.Errorf("expected 2 project_assignments rows, got %d", len(pa.Rows))
	}

	// Round-trip
	resetDB(t, testDB)
	applyFixture(t, testDB, fixture, &ApplyOptions{Force: true, SyncSequences: true})

	if c := rowCount(t, testDB, "workspace_projects"); c != 2 {
		t.Errorf("expected 2 workspace_projects after apply, got %d", c)
	}
	if c := rowCount(t, testDB, "project_assignments"); c != 2 {
		t.Errorf("expected 2 project_assignments after apply, got %d", c)
	}
}
