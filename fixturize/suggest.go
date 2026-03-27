package fixturize

import (
	"fmt"
	"sort"
	"strings"
)

type RootCandidate struct {
	Table       string
	Score       int
	ChildCount  int
	Reachable   int
	OutgoingFKs int
	Reason      string
}

// SuggestRootTables scores tables by how suitable they are as extract roots.
// Tables with zero children are excluded (pure leaves are never useful roots).
func (ds *DatabaseSchema) SuggestRootTables() []RootCandidate {
	// Build inverted graph: parent -> set of child tables
	children := make(map[string]map[string]bool)
	for _, tableName := range ds.GetTables() {
		table, _ := ds.GetTable(tableName)
		for _, fk := range table.ForeignKeys {
			parent := fk.ReferencedTable
			if parent == tableName {
				continue
			}
			if children[parent] == nil {
				children[parent] = make(map[string]bool)
			}
			children[parent][tableName] = true
		}
	}

	var candidates []RootCandidate
	for _, tableName := range ds.GetTables() {
		childCount := len(children[tableName])
		if childCount == 0 {
			continue
		}

		table, _ := ds.GetTable(tableName)
		outgoingFKs := len(table.ForeignKeys)

		reachable, _ := ReachableSubgraph(ds, tableName, 0)
		reachableCount := len(reachable) - 1 // exclude self

		// Scoring
		score := 0

		childScore := childCount * 10
		if childScore > 100 {
			childScore = 100
		}
		score += childScore

		reachScore := reachableCount * 3
		if reachScore > 100 {
			reachScore = 100
		}
		score += reachScore

		score -= outgoingFKs * 5

		// Lookup table penalty: few columns AND no outgoing children beyond this
		if len(table.Columns) <= 4 && childCount == 0 {
			score -= 50
		}

		var reasons []string
		reasons = append(reasons, fmt.Sprintf("%d children", childCount))
		reasons = append(reasons, fmt.Sprintf("%d reachable", reachableCount))
		if outgoingFKs > 0 {
			reasons = append(reasons, fmt.Sprintf("%d outgoing FKs", outgoingFKs))
		}

		candidates = append(candidates, RootCandidate{
			Table:       tableName,
			Score:       score,
			ChildCount:  childCount,
			Reachable:   reachableCount,
			OutgoingFKs: outgoingFKs,
			Reason:      strings.Join(reasons, ", "),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].Table < candidates[j].Table
	})

	return candidates
}

func FormatSuggestRoots(candidates []RootCandidate) string {
	if len(candidates) == 0 {
		return "No root table candidates found (all tables are leaves).\n"
	}

	var b strings.Builder

	// Column widths
	maxTable := len("TABLE")
	for _, c := range candidates {
		if len(c.Table) > maxTable {
			maxTable = len(c.Table)
		}
	}

	fmt.Fprintf(&b, "%-*s  %5s  %8s  %9s  %7s  %s\n",
		maxTable, "TABLE", "SCORE", "CHILDREN", "REACHABLE", "OUT FKs", "REASON")
	b.WriteString(strings.Repeat("-", maxTable+2+5+2+8+2+9+2+7+2+20) + "\n")

	for _, c := range candidates {
		fmt.Fprintf(&b, "%-*s  %5d  %8d  %9d  %7d  %s\n",
			maxTable, c.Table, c.Score, c.ChildCount, c.Reachable, c.OutgoingFKs, c.Reason)
	}

	return b.String()
}
