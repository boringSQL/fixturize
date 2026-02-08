package fixturize

import (
	"fmt"
	"sort"
	"strings"
)

// ReachableSubgraph returns sorted table names reachable from root via FK edges
// in both directions (parents and children). depth==0 means unlimited.
func ReachableSubgraph(schema *DatabaseSchema, root string, depth int) ([]string, error) {
	if !schema.HasTable(root) {
		return nil, fmt.Errorf("root table not found: %s", root)
	}

	// Normalize root name to qualified form
	if !strings.Contains(root, ".") {
		root = "public." + root
	}

	// Build bidirectional adjacency list
	adj := make(map[string]map[string]bool)
	for _, tableName := range schema.GetTables() {
		if adj[tableName] == nil {
			adj[tableName] = make(map[string]bool)
		}
		table, _ := schema.GetTable(tableName)
		for _, fk := range table.ForeignKeys {
			ref := fk.ReferencedTable
			if !schema.HasTable(ref) {
				continue
			}
			adj[tableName][ref] = true
			if adj[ref] == nil {
				adj[ref] = make(map[string]bool)
			}
			adj[ref][tableName] = true
		}
	}

	// BFS
	visited := map[string]bool{root: true}
	queue := []struct {
		name  string
		level int
	}{{root, 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if depth > 0 && cur.level >= depth {
			continue
		}

		for neighbor := range adj[cur.name] {
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, struct {
					name  string
					level int
				}{neighbor, cur.level + 1})
			}
		}
	}

	result := make([]string, 0, len(visited))
	for name := range visited {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

// FormatInspect renders schema in topological order.
func FormatInspect(schema *DatabaseSchema, tables []string) string {
	sorted := schema.TopologicalSort(tables, nil)

	var b strings.Builder
	for i, tableName := range sorted {
		if i > 0 {
			b.WriteByte('\n')
		}

		table, err := schema.GetTable(tableName)
		if err != nil {
			continue
		}

		fmt.Fprintf(&b, "%s\n", tableName)

		cols := sortedColumns(table)

		// alignment
		maxName, maxType := 0, 0
		for _, col := range cols {
			if len(col.Name) > maxName {
				maxName = len(col.Name)
			}
			if len(col.Type) > maxType {
				maxType = len(col.Type)
			}
		}

		for _, col := range cols {
			annotations := formatAnnotations(col)

			fmt.Fprintf(&b, "  %-*s  %-*s", maxName, col.Name, maxType, col.Type)
			if annotations != "" {
				fmt.Fprintf(&b, "  %s", annotations)
			}
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func sortedColumns(table *TableInfo) []*ColumnInfo {
	cols := make([]*ColumnInfo, 0, len(table.Columns))
	for _, col := range table.Columns {
		cols = append(cols, col)
	}
	sort.Slice(cols, func(i, j int) bool {
		return cols[i].OrdinalPosition < cols[j].OrdinalPosition
	})
	return cols
}

func formatAnnotations(col *ColumnInfo) string {
	var parts []string
	if col.IsPrimaryKey {
		parts = append(parts, "PK")
	}
	if col.IsForeignKey && col.ForeignKey != nil {
		parts = append(parts, fmt.Sprintf("FK → %s.%s", col.ForeignKey.ReferencedTable, col.ForeignKey.ReferencedColumn))
	}
	if col.IsUnique {
		parts = append(parts, "UNIQUE")
	}
	if !col.IsNullable {
		parts = append(parts, "NOT NULL")
	}
	return strings.Join(parts, " ")
}
