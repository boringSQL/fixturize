package fixturize

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

type (
	ApplyOptions struct {
		Connection      string
		Fixture         string
		Force           bool
		DryRun          bool
		DisableTriggers bool
		SyncSequences   bool
	}

	ApplyResult struct {
		TablesApplied []string
		RowsInserted  map[string]int
	}

	Applier struct {
		db      *sql.DB
		tx      *sql.Tx
		options *ApplyOptions
		schema  *DatabaseSchema
	}
)

func NewApplier(db *sql.DB, options *ApplyOptions) *Applier {
	return &Applier{
		db:      db,
		options: options,
	}
}

func (a *Applier) Apply(fixture *Fixture) (*ApplyResult, error) {
	fmt.Print("Introspecting schema... ")
	schema, err := IntrospectSchema(a.db)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect schema: %w", err)
	}
	a.schema = schema
	fmt.Printf("%d table(s)\n", len(schema.GetTables()))

	// Get topological order of tables from fixture
	orderedTables := a.getTableOrder(fixture)

	if a.options.Force {
		fmt.Println("Truncating tables...")
		if err := a.truncateTables(orderedTables); err != nil {
			return nil, err
		}
	}

	tx, err := a.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	a.tx = tx

	if _, err := tx.Exec("SET CONSTRAINTS ALL DEFERRED"); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to defer constraints: %w", err)
	}

	if a.options.DisableTriggers {
		if _, err := tx.Exec("SET session_replication_role = 'replica'"); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to disable triggers: %w", err)
		}
	}

	result := &ApplyResult{
		RowsInserted: make(map[string]int),
	}

	for _, tableName := range orderedTables {
		tableData := fixture.Tables[tableName]
		if tableData == nil || len(tableData.Rows) == 0 {
			continue
		}

		fmt.Printf("  Inserting %d rows into %s... ", len(tableData.Rows), shortName(tableName))

		if a.options.DryRun {
			fmt.Println("(dry run)")
			result.TablesApplied = append(result.TablesApplied, tableName)
			result.RowsInserted[tableName] = len(tableData.Rows)
			continue
		}

		inserted, err := a.insertRows(tableName, tableData)
		if err != nil {
			tx.Rollback()
			fmt.Println("FAILED")
			return nil, fmt.Errorf("failed to insert into %s: %w", tableName, err)
		}

		fmt.Println("OK")
		result.TablesApplied = append(result.TablesApplied, tableName)
		result.RowsInserted[tableName] = inserted
	}

	if a.options.DryRun {
		tx.Rollback()
		fmt.Println("Dry run complete, transaction rolled back")
		return result, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	if a.options.SyncSequences {
		fmt.Print("Syncing sequences... ")
		if err := a.syncSequences(result.TablesApplied); err != nil {
			return nil, err
		}
		fmt.Println("OK")
	}

	fmt.Println("Fixture applied successfully")
	return result, nil
}

func (a *Applier) getTableOrder(fixture *Fixture) []string {
	tables := make([]string, 0, len(fixture.Tables))
	for t := range fixture.Tables {
		tables = append(tables, t)
	}

	return a.schema.TopologicalSort(tables, func(t string) {
		fmt.Printf("  Warning: circular FK dependency involving %s (requires DEFERRABLE constraints)\n", shortName(t))
	})
}

func (a *Applier) truncateTables(tables []string) error {
	for i := len(tables) - 1; i >= 0; i-- {
		tableName := tables[i]
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", QuoteQualifiedTable(tableName))
		if _, err := a.db.Exec(query); err != nil {
			return fmt.Errorf("failed to truncate %s: %w", tableName, err)
		}
		fmt.Printf("  Truncated %s\n", shortName(tableName))
	}
	return nil
}

func (a *Applier) insertRows(tableName string, tableData *FixtureTable) (int, error) {
	if len(tableData.Rows) == 0 {
		return 0, nil
	}

	// Check for identity columns
	tableInfo, _ := a.schema.GetTable(tableName)
	hasIdentity := false
	if tableInfo != nil {
		for _, col := range tableData.Columns {
			colInfo := tableInfo.Columns[col]
			if colInfo != nil && colInfo.IsPrimaryKey {
				// Check if it's an identity column
				hasIdentity = a.checkIdentityColumn(tableName, col)
				if hasIdentity {
					break
				}
			}
		}
	}

	colList := QuoteColumns(tableData.Columns)
	placeholders := make([]string, len(tableData.Columns))
	for i := range tableData.Columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	placeholderList := strings.Join(placeholders, ", ")
	quotedTable := QuoteQualifiedTable(tableName)

	var query string
	if hasIdentity {
		query = fmt.Sprintf("INSERT INTO %s (%s) OVERRIDING SYSTEM VALUE VALUES (%s)",
			quotedTable, colList, placeholderList)
	} else {
		query = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quotedTable, colList, placeholderList)
	}

	stmt, err := a.tx.Prepare(query)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, row := range tableData.Rows {
		if len(row) != len(tableData.Columns) {
			return inserted, fmt.Errorf("row has %d values, expected %d columns", len(row), len(tableData.Columns))
		}

		args := make([]any, len(row))
		for i, v := range row {
			args[i] = v
		}

		if _, err := stmt.Exec(args...); err != nil {
			return inserted, fmt.Errorf("insert failed at row %d: %w", inserted+1, err)
		}
		inserted++
	}

	return inserted, nil
}

func (a *Applier) checkIdentityColumn(tableName, colName string) bool {
	schemaName, table := parseTableName(tableName)
	query := `
		SELECT identity_generation
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
	`

	var identityGen *string
	err := a.db.QueryRow(query, schemaName, table, colName).Scan(&identityGen)
	if err != nil {
		return false
	}

	return identityGen != nil && *identityGen == "ALWAYS"
}

func (a *Applier) syncSequences(tables []string) error {
	for _, tableName := range tables {
		tableInfo, err := a.schema.GetTable(tableName)
		if err != nil {
			continue
		}

		for colName, col := range tableInfo.Columns {
			if !col.IsPrimaryKey {
				continue
			}

			var seqName *string
			err := a.db.QueryRow(`SELECT pg_get_serial_sequence($1, $2)`, tableName, colName).Scan(&seqName)
			if err != nil || seqName == nil {
				continue
			}

			_, err = a.db.Exec(fmt.Sprintf(
				`SELECT setval('%s', COALESCE((SELECT MAX(%s) FROM %s), 1), true)`,
				*seqName, QuoteIdent(colName), QuoteQualifiedTable(tableName)))
			if err != nil {
				return fmt.Errorf("failed to sync sequence for %s.%s: %w", tableName, colName, err)
			}
		}
	}
	return nil
}

func ApplyFixtureFile(db *sql.DB, options *ApplyOptions) (*ApplyResult, error) {
	data, err := os.ReadFile(options.Fixture)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixture file: %w", err)
	}

	fixture, err := LoadFixture(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse fixture: %w", err)
	}

	applier := NewApplier(db, options)
	return applier.Apply(fixture)
}
