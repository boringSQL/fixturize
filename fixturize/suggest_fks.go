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
	SuggestFKOptions struct {
		// Validate runs a data probe per candidate; off = name/type match only.
		Validate bool
		// StatementTimeout bounds each probe query, in seconds (0 = 30s).
		StatementTimeout int
		// Verbose prints per-candidate probe progress with timings.
		Verbose bool
	}

	// FKSuggestion is one column that looks like an undeclared foreign key.
	FKSuggestion struct {
		ChildTable   string
		ChildColumn  string
		ParentTable  string // empty when no candidate parent was found
		ParentColumn string
		// Checked is true once the data probe ran for this suggestion.
		Checked bool
		// OrphanCount is how many child rows would fail the FK (0 = clean).
		OrphanCount int64
	}
)

func SuggestMissingFKs(schema *DatabaseSchema, tables []string) []FKSuggestion {
	// index every table by its short (unqualified) name for parent lookup
	byShortName := make(map[string][]string)
	for _, qualified := range schema.GetTables() {
		short := shortName(qualified)
		byShortName[short] = append(byShortName[short], qualified)
	}

	var suggestions []FKSuggestion

	sorted := append([]string(nil), tables...)
	sort.Strings(sorted)

	for _, tableName := range sorted {
		table, err := schema.GetTable(tableName)
		if err != nil {
			continue
		}
		qualified := table.Schema + "." + table.Name

		for _, col := range sortedColumns(table) {
			// already constrained — skip
			if col.IsForeignKey {
				continue
			}
			// lone PK is a surrogate identity, not a reference
			if col.IsPrimaryKey && len(table.PrimaryKey) == 1 {
				continue
			}
			base, ok := fkColumnBase(col.Name)
			if !ok {
				continue
			}

			s := FKSuggestion{ChildTable: qualified, ChildColumn: col.Name}

			parent, parentCol := matchParent(schema, byShortName, table.Schema, base, col.Type)
			if parent != "" {
				s.ParentTable = parent
				s.ParentColumn = parentCol
			}
			suggestions = append(suggestions, s)
		}
	}

	return suggestions
}

func fkColumnBase(name string) (string, bool) {
	lower := strings.ToLower(name)
	if !strings.HasSuffix(lower, "_id") {
		return "", false
	}
	base := lower[:len(lower)-len("_id")]
	if base == "" {
		return "", false
	}
	return base, true
}

// matchParent pairs base with a same-named parent whose single-column PK is
// type-compatible; same-schema tables win ties.
func matchParent(schema *DatabaseSchema, byShortName map[string][]string, childSchema, base, childType string) (table, column string) {
	for _, form := range parentNameForms(base) {
		candidates := byShortName[form]
		if len(candidates) == 0 {
			continue
		}
		// prefer a table in the same schema as the child
		sort.Slice(candidates, func(i, j int) bool {
			si, _ := parseTableName(candidates[i])
			sj, _ := parseTableName(candidates[j])
			if (si == childSchema) != (sj == childSchema) {
				return si == childSchema
			}
			return candidates[i] < candidates[j]
		})

		for _, cand := range candidates {
			parent, err := schema.GetTable(cand)
			if err != nil || len(parent.PrimaryKey) != 1 {
				continue
			}
			pkCol := parent.Columns[parent.PrimaryKey[0]]
			if pkCol == nil {
				continue
			}
			if typesCompatible(childType, pkCol.Type) {
				return cand, pkCol.Name
			}
		}
	}
	return "", ""
}

// parentNameForms expands a singular base into likely table names — common
// English plurals only.
func parentNameForms(base string) []string {
	forms := []string{base, base + "s"}
	switch {
	case strings.HasSuffix(base, "y"):
		forms = append(forms, base[:len(base)-1]+"ies")
	case strings.HasSuffix(base, "s"), strings.HasSuffix(base, "x"),
		strings.HasSuffix(base, "z"), strings.HasSuffix(base, "ch"),
		strings.HasSuffix(base, "sh"):
		forms = append(forms, base+"es")
	}
	return forms
}

// typesCompatible reports whether an FK could link the two types; modifiers
// dropped, integer family unified.
func typesCompatible(a, b string) bool {
	return normalizeType(a) == normalizeType(b)
}

func normalizeType(t string) string {
	if i := strings.IndexByte(t, '('); i >= 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(strings.ToLower(t))
	switch t {
	case "smallint", "integer", "bigint", "int2", "int4", "int8":
		return "integer"
	}
	return t
}

// ValidateFKSuggestions counts orphaned child rows per matched suggestion, in
// a read-only REPEATABLE READ transaction.
func ValidateFKSuggestions(ctx context.Context, db *pgxpool.Pool, suggestions []FKSuggestion, opts SuggestFKOptions) error {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	timeout := opts.StatementTimeout
	if timeout <= 0 {
		timeout = 30
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET statement_timeout = '%ds'", timeout)); err != nil {
		return fmt.Errorf("failed to set statement_timeout: %w", err)
	}

	if opts.Verbose {
		fmt.Printf("Probing %d candidate FK(s)...\n", countMatched(suggestions))
	}

	for i := range suggestions {
		s := &suggestions[i]
		if s.ParentTable == "" {
			continue
		}
		started := time.Now()

		// reuse the check command's orphan query
		c := FKConstraint{
			ChildTable:  s.ChildTable,
			ParentTable: s.ParentTable,
			ChildCols:   []string{s.ChildColumn},
			ParentCols:  []string{s.ParentColumn},
		}
		query := fmt.Sprintf("SELECT count(*) FROM %s AS c WHERE %s",
			QuoteQualifiedTable(s.ChildTable), orphanWhere(c))

		var count int64
		if err := tx.QueryRow(ctx, query).Scan(&count); err != nil {
			return fmt.Errorf("validation probe for %s.%s failed: %w", s.ChildTable, s.ChildColumn, err)
		}
		s.OrphanCount = count
		s.Checked = true

		if opts.Verbose {
			status := "would validate"
			if count > 0 {
				status = fmt.Sprintf("%d row(s) without parent", count)
			}
			fmt.Printf("  %s.%s → %s.%s ... %s [%s]\n",
				s.ChildTable, s.ChildColumn, s.ParentTable, s.ParentColumn, status, elapsed(started))
		}
	}

	return nil
}

func countMatched(suggestions []FKSuggestion) int {
	n := 0
	for _, s := range suggestions {
		if s.ParentTable != "" {
			n++
		}
	}
	return n
}

// FormatSuggestFKs renders the report grouped by child table; showUnmatched
// also lists *_id columns with no candidate parent.
func FormatSuggestFKs(suggestions []FKSuggestion, showUnmatched bool) string {
	byTable := make(map[string][]FKSuggestion)
	var tableOrder []string
	for _, s := range suggestions {
		if s.ParentTable == "" && !showUnmatched {
			continue
		}
		if _, ok := byTable[s.ChildTable]; !ok {
			tableOrder = append(tableOrder, s.ChildTable)
		}
		byTable[s.ChildTable] = append(byTable[s.ChildTable], s)
	}

	var (
		b         strings.Builder
		safe      int
		withData  int
		unmatched int
	)

	for i, table := range tableOrder {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s\n", table)

		group := byTable[table]
		maxCol := 0
		for _, s := range group {
			if len(s.ChildColumn) > maxCol {
				maxCol = len(s.ChildColumn)
			}
		}

		for _, s := range group {
			if s.ParentTable == "" {
				unmatched++
				fmt.Fprintf(&b, "  %-*s  (no candidate parent table)\n", maxCol, s.ChildColumn)
				continue
			}
			target := fmt.Sprintf("→ %s.%s", s.ParentTable, s.ParentColumn)
			switch {
			case !s.Checked:
				fmt.Fprintf(&b, "  %-*s  %-28s  candidate (not validated)\n", maxCol, s.ChildColumn, target)
			case s.OrphanCount == 0:
				safe++
				fmt.Fprintf(&b, "  %-*s  %-28s  ✓ would validate — safe to add\n", maxCol, s.ChildColumn, target)
			default:
				withData++
				fmt.Fprintf(&b, "  %-*s  %-28s  ✗ %d row(s) have no parent\n", maxCol, s.ChildColumn, target, s.OrphanCount)
			}
		}
	}

	b.WriteByte('\n')
	matched := safe + withData
	if !suggestionsHaveChecks(suggestions) {
		fmt.Fprintf(&b, "%d candidate FK(s)", matched)
	} else {
		fmt.Fprintf(&b, "%d candidate FK(s): %d safe to add, %d with orphaned data", matched, safe, withData)
	}
	if showUnmatched && unmatched > 0 {
		fmt.Fprintf(&b, "; %d unmatched *_id column(s)", unmatched)
	}
	b.WriteString(".\n")

	return b.String()
}

func suggestionsHaveChecks(suggestions []FKSuggestion) bool {
	for _, s := range suggestions {
		if s.Checked {
			return true
		}
	}
	return false
}

// FormatSuggestFKsSQL emits one ALTER TABLE per matched candidate; ones with
// orphaned data get NOT VALID.
func FormatSuggestFKsSQL(suggestions []FKSuggestion) string {
	var b strings.Builder
	b.WriteString("-- Missing foreign keys suggested by fixturize suggest-fks.\n")
	b.WriteString("-- Review before running — column names are a heuristic, not proof.\n")

	found := false
	for _, s := range suggestions {
		if s.ParentTable == "" {
			continue
		}
		found = true
		constraint := fmt.Sprintf("%s_%s_fkey", shortName(s.ChildTable), s.ChildColumn)

		var note, notValid string
		switch {
		case s.Checked && s.OrphanCount > 0:
			note = fmt.Sprintf(" (%d orphan row(s) — NOT VALID)", s.OrphanCount)
			notValid = " NOT VALID"
		case s.Checked:
			note = " (would validate)"
		default:
			note = " (data not validated)"
		}

		fmt.Fprintf(&b, "\n-- %s.%s → %s.%s%s\n",
			s.ChildTable, s.ChildColumn, s.ParentTable, s.ParentColumn, note)
		fmt.Fprintf(&b, "ALTER TABLE %s\n  ADD CONSTRAINT %s\n  FOREIGN KEY (%s) REFERENCES %s (%s)%s;\n",
			QuoteQualifiedTable(s.ChildTable), QuoteIdent(constraint),
			QuoteIdent(s.ChildColumn), QuoteQualifiedTable(s.ParentTable),
			QuoteIdent(s.ParentColumn), notValid)
	}

	if !found {
		b.WriteString("\n-- No candidate foreign keys found.\n")
	}

	return b.String()
}
