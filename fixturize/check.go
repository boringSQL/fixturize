package fixturize

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	CheckOptions struct {
		// Samples caps how many orphan rows are fetched per constraint.
		Samples int
		// StatementTimeout bounds each query, in seconds (0 = 30s default).
		StatementTimeout int
		// Verbose prints per-constraint progress with timings.
		Verbose bool
	}

	OrphanResult struct {
		Constraint    FKConstraint
		OrphanCount   int64
		SampleColumns []string
		Samples       []map[string]any
	}
)

func collectFKConstraints(schema *DatabaseSchema, tables []string) []FKConstraint {
	var result []FKConstraint

	for _, name := range tables {
		table, err := schema.GetTable(name)
		if err != nil {
			continue
		}
		qualified := table.Schema + "." + table.Name

		groups := make(map[string]*FKConstraint)
		var order []string
		for _, fk := range table.ForeignKeys {
			g, ok := groups[fk.ConstraintName]
			if !ok {
				g = &FKConstraint{
					ConstraintName: fk.ConstraintName,
					ChildTable:     qualified,
					ParentTable:    fk.ReferencedTable,
				}
				groups[fk.ConstraintName] = g
				order = append(order, fk.ConstraintName)
			}
			g.ChildCols = append(g.ChildCols, fk.ColumnName)
			g.ParentCols = append(g.ParentCols, fk.ReferencedColumn)
		}

		for _, cn := range order {
			result = append(result, *groups[cn])
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].ChildTable != result[j].ChildTable {
			return result[i].ChildTable < result[j].ChildTable
		}
		return result[i].ConstraintName < result[j].ConstraintName
	})

	return result
}

func sampleColumns(schema *DatabaseSchema, c FKConstraint) []string {
	var cols []string
	seen := make(map[string]bool)

	if table, err := schema.GetTable(c.ChildTable); err == nil {
		for _, pk := range table.PrimaryKey {
			if !seen[pk] {
				seen[pk] = true
				cols = append(cols, pk)
			}
		}
	}
	for _, fc := range c.ChildCols {
		if !seen[fc] {
			seen[fc] = true
			cols = append(cols, fc)
		}
	}

	return cols
}

func CheckOrphans(ctx context.Context, db *pgxpool.Pool, schema *DatabaseSchema, tables []string, opts CheckOptions) ([]OrphanResult, error) {
	constraints := collectFKConstraints(schema, tables)

	tx, err := db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	timeout := opts.StatementTimeout
	if timeout <= 0 {
		timeout = 30
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET statement_timeout = '%ds'", timeout)); err != nil {
		return nil, fmt.Errorf("failed to set statement_timeout: %w", err)
	}

	if opts.Verbose {
		fmt.Printf("Checking %d FK constraint(s)...\n", len(constraints))
	}

	results := make([]OrphanResult, 0, len(constraints))
	for _, c := range constraints {
		where := orphanWhere(c)
		started := time.Now()

		var count int64
		countQuery := fmt.Sprintf("SELECT count(*) FROM %s AS c WHERE %s",
			QuoteQualifiedTable(c.ChildTable), where)
		if err := tx.QueryRow(ctx, countQuery).Scan(&count); err != nil {
			return nil, fmt.Errorf("orphan check for %s.%s failed: %w", c.ChildTable, c.ConstraintName, err)
		}

		res := OrphanResult{Constraint: c, OrphanCount: count}

		if count > 0 && opts.Samples > 0 {
			res.SampleColumns = sampleColumns(schema, c)
			selectList := make([]string, len(res.SampleColumns))
			for i, col := range res.SampleColumns {
				selectList[i] = "c." + QuoteIdent(col)
			}
			sampleQuery := fmt.Sprintf("SELECT %s FROM %s AS c WHERE %s LIMIT %d",
				strings.Join(selectList, ", "), QuoteQualifiedTable(c.ChildTable), where, opts.Samples)

			rows, err := tx.Query(ctx, sampleQuery)
			if err != nil {
				return nil, fmt.Errorf("orphan sample for %s.%s failed: %w", c.ChildTable, c.ConstraintName, err)
			}
			for rows.Next() {
				vals, err := rows.Values()
				if err != nil {
					rows.Close()
					return nil, err
				}
				row := make(map[string]any, len(res.SampleColumns))
				for i, col := range res.SampleColumns {
					row[col] = vals[i]
				}
				res.Samples = append(res.Samples, row)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}

		if opts.Verbose {
			status := "ok"
			if count > 0 {
				status = fmt.Sprintf("%d orphan(s)", count)
			}
			fmt.Printf("  %s.%s ... %s [%s]\n",
				c.ChildTable, c.ConstraintName, status, elapsed(started))
		}

		results = append(results, res)
	}

	return results, nil
}

func TotalOrphans(results []OrphanResult) int64 {
	var total int64
	for _, r := range results {
		total += r.OrphanCount
	}
	return total
}

func FormatCheck(results []OrphanResult) string {
	byTable := make(map[string][]OrphanResult)
	var tableOrder []string
	for _, r := range results {
		t := r.Constraint.ChildTable
		if _, ok := byTable[t]; !ok {
			tableOrder = append(tableOrder, t)
		}
		byTable[t] = append(byTable[t], r)
	}

	var (
		b              strings.Builder
		totalOrphans   int64
		badConstraints int
		badTables      = make(map[string]bool)
	)

	for i, table := range tableOrder {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s\n", table)

		group := byTable[table]
		maxName := 0
		for _, r := range group {
			if len(r.Constraint.ConstraintName) > maxName {
				maxName = len(r.Constraint.ConstraintName)
			}
		}

		for _, r := range group {
			label := fkArrow(r.Constraint)
			if r.OrphanCount == 0 {
				fmt.Fprintf(&b, "  ✓ %-*s  %s\n", maxName, r.Constraint.ConstraintName, label)
				continue
			}

			totalOrphans += r.OrphanCount
			badConstraints++
			badTables[table] = true

			fmt.Fprintf(&b, "  ✗ %-*s  %s  %d orphan row(s)\n",
				maxName, r.Constraint.ConstraintName, label, r.OrphanCount)
			for _, s := range r.Samples {
				fmt.Fprintf(&b, "      %s\n", formatSample(r.SampleColumns, s))
			}
			if remaining := r.OrphanCount - int64(len(r.Samples)); remaining > 0 && len(r.Samples) > 0 {
				fmt.Fprintf(&b, "      ... and %d more\n", remaining)
			}
		}
	}

	b.WriteByte('\n')
	if totalOrphans == 0 {
		fmt.Fprintf(&b, "No orphaned rows. All %d FK constraint(s) satisfied.\n", len(results))
	} else {
		fmt.Fprintf(&b, "%d orphan row(s) in %d of %d constraint(s) across %d table(s).\n",
			totalOrphans, badConstraints, len(results), len(badTables))
	}

	return b.String()
}

func FormatCheckSQL(results []OrphanResult) string {
	var b strings.Builder
	b.WriteString("-- Orphan cleanup generated by fixturize check.\n")
	b.WriteString("-- Review before running: each DELETE removes child rows whose FK\n")
	b.WriteString("-- columns reference a parent row that does not exist.\n")

	found := false
	for _, r := range results {
		if r.OrphanCount == 0 {
			continue
		}
		found = true
		c := r.Constraint
		where := strings.Replace(orphanWhere(c), " AND NOT EXISTS", "\n  AND NOT EXISTS", 1)
		fmt.Fprintf(&b, "\n-- %s %s: %d orphan row(s)\n", c.ChildTable, c.ConstraintName, r.OrphanCount)
		fmt.Fprintf(&b, "DELETE FROM %s AS c\nWHERE %s;\n", QuoteQualifiedTable(c.ChildTable), where)
	}

	if !found {
		b.WriteString("\n-- No orphaned rows found.\n")
	}

	return b.String()
}

func fkArrow(c FKConstraint) string {
	return fmt.Sprintf("%s → %s.%s", joinCols(c.ChildCols), c.ParentTable, joinCols(c.ParentCols))
}

func joinCols(cols []string) string {
	if len(cols) == 1 {
		return cols[0]
	}
	return "(" + strings.Join(cols, ",") + ")"
}

func formatSample(cols []string, row map[string]any) string {
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = col + "=" + formatSampleValue(row[col])
	}
	return strings.Join(parts, "  ")
}

func formatSampleValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
