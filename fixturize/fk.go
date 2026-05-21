package fixturize

import (
	"fmt"
	"strings"
	"time"
)

// FKConstraint groups every column of a single FK constraint; composite FKs
// arrive from introspection as multiple rows sharing a constraint name and are
// collapsed into one here.
type FKConstraint struct {
	ConstraintName string
	ChildTable     string
	ParentTable    string
	ChildCols      []string
	ParentCols     []string
}

// orphanWhere builds the WHERE predicate matching orphaned child rows; aliases
// child = c, parent = p. MATCH SIMPLE exempts rows with any NULL FK column, so
// only fully-populated rows are flagged.
func orphanWhere(c FKConstraint) string {
	conds := make([]string, 0, len(c.ChildCols)+1)
	for _, col := range c.ChildCols {
		conds = append(conds, "c."+QuoteIdent(col)+" IS NOT NULL")
	}

	joins := make([]string, len(c.ChildCols))
	for i := range c.ChildCols {
		joins[i] = fmt.Sprintf("p.%s = c.%s",
			QuoteIdent(c.ParentCols[i]), QuoteIdent(c.ChildCols[i]))
	}

	conds = append(conds, fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s p WHERE %s)",
		QuoteQualifiedTable(c.ParentTable), strings.Join(joins, " AND ")))

	return strings.Join(conds, " AND ")
}

// elapsed formats a duration compactly: sub-second as ms, otherwise seconds.
func elapsed(since time.Time) string {
	d := time.Since(since)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
