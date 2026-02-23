package fixturize

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
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

func (a *Applier) Apply(ctx context.Context, fixture *Fixture) (*ApplyResult, error) {
	fmt.Print("Introspecting schema... ")
	schema, err := IntrospectSchema(a.db)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect schema: %w", err)
	}
	a.schema = schema
	fmt.Printf("%d table(s)\n", len(schema.GetTables()))

	orderedTables := a.getTableOrder(fixture)

	if a.options.Force {
		fmt.Println("Truncating tables...")
		if err := a.truncateTables(orderedTables); err != nil {
			return nil, err
		}
	}

	result := &ApplyResult{
		RowsInserted: make(map[string]int),
	}

	if a.options.DryRun {
		for _, tableName := range orderedTables {
			tableData := fixture.Tables[tableName]
			if tableData == nil || len(tableData.Rows) == 0 {
				continue
			}

			fmt.Printf("  Inserting %d rows into %s... (dry run)\n", len(tableData.Rows), shortName(tableName))

			result.TablesApplied = append(result.TablesApplied, tableName)
			result.RowsInserted[tableName] = len(tableData.Rows)
		}
		fmt.Println("Dry run complete, transaction rolled back")
		return result, nil
	}

	// use a raw pgx connection for COPY-based bulk insert
	sqlConn, err := a.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer sqlConn.Close()

	err = sqlConn.Raw(func(driverConn any) error {
		pgxConn := driverConn.(*stdlib.Conn).Conn()
		tx, err := pgxConn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, "SET CONSTRAINTS ALL DEFERRED"); err != nil {
			return fmt.Errorf("failed to defer constraints: %w", err)
		}

		if a.options.DisableTriggers {
			if _, err := tx.Exec(ctx, "SET session_replication_role = 'replica'"); err != nil {
				return fmt.Errorf("failed to disable triggers: %w", err)
			}
		}

		for _, tableName := range orderedTables {
			tableData := fixture.Tables[tableName]
			if tableData == nil || len(tableData.Rows) == 0 {
				continue
			}

			fmt.Printf("  Inserting %d rows into %s... ", len(tableData.Rows), shortName(tableName))

			schemaName, table := parseTableName(tableName)
			quotedCols := make([]string, len(tableData.Columns))
			for i, c := range tableData.Columns {
				quotedCols[i] = pgx.Identifier{c}.Sanitize()
			}
			copySQL := fmt.Sprintf("COPY %s (%s) FROM STDIN",
				pgx.Identifier{schemaName, table}.Sanitize(),
				strings.Join(quotedCols, ", "))

			var buf bytes.Buffer
			for _, row := range tableData.Rows {
				for i, v := range row {
					if i > 0 {
						buf.WriteByte('\t')
					}
					buf.WriteString(formatCopyValue(v))
				}
				buf.WriteByte('\n')
			}

			// tx.Conn().PgConn().CopyFrom to be used instead high-level tx.CopyFrom
			// whcih uses binary protocol and running into the serialization issues
			tag, err := tx.Conn().PgConn().CopyFrom(ctx, &buf, copySQL)
			copied := tag.RowsAffected()
			if err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("failed to copy into %s: %w", tableName, err)
			}

			fmt.Println("OK")
			result.TablesApplied = append(result.TablesApplied, tableName)
			result.RowsInserted[tableName] = int(copied)
		}

		return tx.Commit(ctx)
	})
	if err != nil {
		return nil, err
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

func formatCopyValue(v any) string {
	if v == nil {
		return `\N`
	}
	switch val := v.(type) {
	case string:
		return escapeCopyText(val)
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	default:
		b, _ := json.Marshal(val)
		return escapeCopyText(string(b))
	}
}

func escapeCopyText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
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
	return applier.Apply(context.Background(), fixture)
}
