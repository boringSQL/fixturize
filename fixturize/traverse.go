package fixturize

import (
	"fmt"
	"strings"
)

func (e *Extractor) buildInvertedGraph() {
	for _, tableName := range e.schema.GetTables() {
		tableInfo, _ := e.schema.GetTable(tableName)
		for _, fk := range tableInfo.ForeignKeys {
			parentTable := fk.ReferencedTable
			e.invertedGraph[parentTable] = append(e.invertedGraph[parentTable], fkEdge{
				ConstraintName: fk.ConstraintName,
				ChildTable:     tableName,
				ChildColumn:    fk.ColumnName,
				ParentColumn:   fk.ReferencedColumn,
			})
		}
	}
}

func (e *Extractor) extractRootRows(table, clause string) error {
	cols, err := e.selectColumns(table)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("SELECT %s FROM %s", cols, QuoteQualifiedTable(table))
	if clause != "" {
		query += " " + clause
	}

	rows, err := e.tx.Query(query)
	if err != nil {
		return fmt.Errorf("root query failed: %w", err)
	}
	defer rows.Close()

	return e.collectRows(table, rows)
}

func (e *Extractor) extractAllRows(table string) error {
	cols, err := e.selectColumns(table)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("SELECT %s FROM %s", cols, QuoteQualifiedTable(table))

	rows, err := e.tx.Query(query)
	if err != nil {
		return fmt.Errorf("query failed for %s: %w", table, err)
	}
	defer rows.Close()

	return e.collectRows(table, rows)
}

func (e *Extractor) extractParents() (bool, error) {
	foundNew := false

	for tableName, tableRows := range e.collected {
		tableInfo, err := e.schema.GetTable(tableName)
		if err != nil {
			continue
		}

		// composite FKs show up as multiple ForeignKeyInfo rows with the same
		// constraint name. They shoul be grouped to avoid separate single-column
		// lookups
		type fkGroup struct {
			parentTable string
			fks         []*ForeignKeyInfo
		}
		groups := make(map[string]*fkGroup)
		var groupOrder []string
		for _, fk := range tableInfo.ForeignKeys {
			g, ok := groups[fk.ConstraintName]
			if !ok {
				g = &fkGroup{parentTable: fk.ReferencedTable}
				groups[fk.ConstraintName] = g
				groupOrder = append(groupOrder, fk.ConstraintName)
			}
			g.fks = append(g.fks, fk)
		}

		for _, constraintName := range groupOrder {
			g := groups[constraintName]
			parentTable := g.parentTable

			if e.excludeSet[shortName(parentTable)] || e.excludeSet[parentTable] {
				e.warnExcludedParent(parentTable, tableName)
				continue
			}

			if e.limitReached[parentTable] {
				continue
			}

			parentCols, err := e.selectColumns(parentTable)
			if err != nil {
				return false, err
			}

			var query string
			var queryParams []any
			var tupleCount int

			if len(g.fks) == 1 {
				// single-column FK: use = ANY($1) which PG handles efficiently
				fk := g.fks[0]
				var fkValues []any
				for _, row := range tableRows {
					val, ok := row[fk.ColumnName]
					if !ok || val == nil {
						continue
					}
					if e.collectedPKs[parentTable] != nil && e.collectedPKs[parentTable][val] {
						continue
					}
					fkValues = append(fkValues, val)
				}
				if len(fkValues) == 0 {
					continue
				}
				fkValues = uniqueValues(fkValues)
				tupleCount = len(fkValues)

				query = fmt.Sprintf("SELECT %s FROM %s WHERE %s = ANY($1)",
					parentCols, QuoteQualifiedTable(parentTable), QuoteIdent(fk.ReferencedColumn))
				queryParams = []any{fkValues}
			} else {
				// composite FK: collect tuples and match with OR'd equality conditions
				var tuples [][]any
				for _, row := range tableRows {
					allPresent := true
					tuple := make([]any, len(g.fks))
					for i, fk := range g.fks {
						val, ok := row[fk.ColumnName]
						if !ok || val == nil {
							allPresent = false
							break
						}
						tuple[i] = val
					}
					if !allPresent {
						continue
					}
					tuples = append(tuples, tuple)
				}
				tuples = uniqueTuples(tuples)
				if len(tuples) == 0 {
					continue
				}
				tupleCount = len(tuples)

				var refCols []string
				for _, fk := range g.fks {
					refCols = append(refCols, QuoteIdent(fk.ReferencedColumn))
				}
				query, queryParams = buildTupleQuery(
					fmt.Sprintf("SELECT %s FROM %s", parentCols, QuoteQualifiedTable(parentTable)),
					refCols, tuples)
			}

			rows, err := e.tx.Query(query, queryParams...)
			if err != nil {
				return false, fmt.Errorf("parent query for %s (constraint %s) failed: %w", parentTable, constraintName, err)
			}

			beforeCount := len(e.collected[parentTable])
			if err := e.collectRows(parentTable, rows); err != nil {
				rows.Close()
				return false, err
			}
			rows.Close()

			afterCount := len(e.collected[parentTable])
			if afterCount > beforeCount {
				foundNew = true
				newRows := afterCount - beforeCount
				if e.options.Verbose {
					fmt.Printf("  ↑ %s... %d row(s) [%s.%s → %s.%s, %d FKs]\n",
						shortName(parentTable), newRows,
						shortName(tableName), fkColNames(g.fks, true),
						shortName(parentTable), fkColNames(g.fks, false),
						tupleCount)
				} else {
					fmt.Printf("  ↑ %s... %d row(s) (parent of %s)\n", shortName(parentTable), newRows, shortName(tableName))
				}
			}
		}
	}

	return foundNew, nil
}

func (e *Extractor) extractChildren() (bool, error) {
	foundNew := false

	type bfsItem struct {
		table string
		depth int
	}

	// only seed BFS from tables we discovered going downward (root, includes,
	// and their children).
	// From parents the tables pulled in as parents are for FK integrity
	// only
	visited := make(map[string]bool)
	var queue []bfsItem
	for t := range e.downwardTables {
		visited[t] = true
		queue = append(queue, bfsItem{table: t, depth: 0})
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if e.options.Depth > 0 && item.depth >= e.options.Depth {
			if len(e.invertedGraph[item.table]) > 0 {
				e.warn("depth limit %d reached at %s; child tables not traversed", e.options.Depth, shortName(item.table))
			}
			continue
		}

		// same composite FK grouping as in extractParents — edges sharing a
		// constraint name belong to the same multi-column FK and need a tuple query
		type edgeGroupKey struct {
			constraintName string
			childTable     string
		}
		edgeGroups := make(map[edgeGroupKey][]fkEdge)
		var edgeGroupOrder []edgeGroupKey
		for _, edge := range e.invertedGraph[item.table] {
			key := edgeGroupKey{edge.ConstraintName, edge.ChildTable}
			if _, ok := edgeGroups[key]; !ok {
				edgeGroupOrder = append(edgeGroupOrder, key)
			}
			edgeGroups[key] = append(edgeGroups[key], edge)
		}

		for _, gKey := range edgeGroupOrder {
			groupEdges := edgeGroups[gKey]
			childTable := gKey.childTable

			if e.excludeSet[shortName(childTable)] || e.excludeSet[childTable] {
				continue
			}

			if e.limitReached[childTable] {
				continue
			}

			parentRows := e.collected[item.table]
			if len(parentRows) == 0 {
				continue
			}

			childSelectCols, err := e.selectColumns(childTable)
			if err != nil {
				return false, err
			}

			var query string
			var queryParams []any
			var tupleCount int

			if len(groupEdges) == 1 {
				// single-column FK: use = ANY($1)
				edge := groupEdges[0]
				var pkValues []any
				for _, row := range parentRows {
					val, ok := row[edge.ParentColumn]
					if !ok || val == nil {
						continue
					}
					pkValues = append(pkValues, val)
				}
				if len(pkValues) == 0 {
					continue
				}
				pkValues = uniqueValues(pkValues)
				tupleCount = len(pkValues)

				query = fmt.Sprintf("SELECT %s FROM %s WHERE %s = ANY($1) AND %s IS NOT NULL",
					childSelectCols, QuoteQualifiedTable(childTable),
					QuoteIdent(edge.ChildColumn), QuoteIdent(edge.ChildColumn))
				queryParams = []any{pkValues}
			} else {
				// composite FK: collect tuples and match with OR'd equality conditions
				var tuples [][]any
				for _, row := range parentRows {
					allPresent := true
					tuple := make([]any, len(groupEdges))
					for i, edge := range groupEdges {
						val, ok := row[edge.ParentColumn]
						if !ok || val == nil {
							allPresent = false
							break
						}
						tuple[i] = val
					}
					if !allPresent {
						continue
					}
					tuples = append(tuples, tuple)
				}
				tuples = uniqueTuples(tuples)
				if len(tuples) == 0 {
					continue
				}
				tupleCount = len(tuples)

				var childFKCols []string
				for _, edge := range groupEdges {
					childFKCols = append(childFKCols, QuoteIdent(edge.ChildColumn))
				}
				query, queryParams = buildTupleQuery(
					fmt.Sprintf("SELECT %s FROM %s", childSelectCols, QuoteQualifiedTable(childTable)),
					childFKCols, tuples)
			}

			if filterExpr, ok := e.filters[childTable]; ok {
				query += fmt.Sprintf(" AND (%s)", filterExpr)
			}

			if e.options.Limit > 0 {
				query += fmt.Sprintf(" LIMIT %d", e.options.Limit)
			}

			rows, err := e.tx.Query(query, queryParams...)
			if err != nil {
				return false, fmt.Errorf("child query for %s failed: %w", childTable, err)
			}

			beforeCount := len(e.collected[childTable])
			if err := e.collectRows(childTable, rows); err != nil {
				rows.Close()
				return false, err
			}
			rows.Close()

			afterCount := len(e.collected[childTable])

			if e.options.Limit > 0 && afterCount >= e.options.Limit {
				e.limitReached[childTable] = true
				e.warn("table %s limited to %d rows, subgraph may be incomplete", shortName(childTable), e.options.Limit)
			}

			if afterCount > beforeCount {
				foundNew = true
				newRows := afterCount - beforeCount
				if e.options.Verbose {
					fmt.Printf("  ↓ %s... %d row(s) [depth=%d, %s.%s → %s.%s, %d PKs]\n",
						shortName(childTable), newRows, item.depth+1,
						shortName(item.table), edgeColNames(groupEdges, true),
						shortName(childTable), edgeColNames(groupEdges, false),
						tupleCount)
				} else {
					fmt.Printf("  ↓ %s... %d row(s)\n", shortName(childTable), newRows)
				}
			}

			if err := e.extractSelfReferencing(childTable); err != nil {
				return false, err
			}

			if !visited[childTable] {
				visited[childTable] = true
				e.downwardTables[childTable] = true
				queue = append(queue, bfsItem{table: childTable, depth: item.depth + 1})
			}
		}
	}

	return foundNew, nil
}

func (e *Extractor) extractSelfReferencing(table string) error {
	tableInfo, err := e.schema.GetTable(table)
	if err != nil {
		return nil
	}

	qualifiedName := tableInfo.Schema + "." + tableInfo.Name

	var selfFKs []*ForeignKeyInfo
	for _, fk := range tableInfo.ForeignKeys {
		if fk.ReferencedTable == qualifiedName {
			selfFKs = append(selfFKs, fk)
		}
	}

	if len(selfFKs) == 0 {
		return nil
	}

	for iteration := 0; iteration < maxTraversalIterations; iteration++ {
		foundAny := false

		for _, fk := range selfFKs {
			var pkValues []any
			for _, row := range e.collected[table] {
				for _, pkCol := range tableInfo.PrimaryKey {
					if val, ok := row[pkCol]; ok && val != nil {
						pkValues = append(pkValues, val)
					}
				}
			}

			if len(pkValues) == 0 {
				continue
			}

			pkValues = uniqueValues(pkValues)

			selfCols, err := e.selectColumns(table)
			if err != nil {
				return err
			}
			query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ANY($1) AND %s IS NOT NULL",
				selfCols, QuoteQualifiedTable(table), QuoteIdent(fk.ColumnName), QuoteIdent(fk.ColumnName))

			if filterExpr, ok := e.filters[table]; ok {
				query += fmt.Sprintf(" AND (%s)", filterExpr)
			}

			rows, err := e.tx.Query(query, pkValues)
			if err != nil {
				return fmt.Errorf("self-referencing query for %s failed: %w", table, err)
			}

			before := len(e.collected[table])
			if err := e.collectRows(table, rows); err != nil {
				rows.Close()
				return err
			}
			rows.Close()

			if len(e.collected[table]) > before {
				foundAny = true
			}
		}

		if !foundAny {
			break
		}
	}

	return nil
}

// buildTupleQuery generates (col1 = $1 AND col2 = $2) OR (col1 = $3 AND col2 = $4) ...
// Can't use WHERE (col1, col2) IN (VALUES ($1,$2), ...) because PG defaults untyped
// params to text and end with with "operator does not exist: integer = text". the OR
// form lets PG infer types from the column definitions instead.
func buildTupleQuery(selectClause string, cols []string, tuples [][]any) (string, []any) {
	var params []any
	var orClauses []string
	for _, tuple := range tuples {
		var andParts []string
		for i, val := range tuple {
			params = append(params, val)
			andParts = append(andParts, fmt.Sprintf("%s = $%d", cols[i], len(params)))
		}
		orClauses = append(orClauses, "("+strings.Join(andParts, " AND ")+")")
	}
	query := fmt.Sprintf("%s WHERE (%s)", selectClause, strings.Join(orClauses, " OR "))
	return query, params
}

func uniqueValues(vals []any) []any {
	seen := make(map[any]bool, len(vals))
	result := make([]any, 0, len(vals))
	for _, v := range vals {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func uniqueTuples(tuples [][]any) [][]any {
	seen := make(map[string]bool, len(tuples))
	var result [][]any
	for _, t := range tuples {
		key := buildPKKeyFromSlice(t)
		if !seen[key] {
			seen[key] = true
			result = append(result, t)
		}
	}
	return result
}

func buildPKKeyFromSlice(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%v", v)
	}
	return strings.Join(parts, "|")
}

func fkColNames(fks []*ForeignKeyInfo, local bool) string {
	if len(fks) == 1 {
		if local {
			return fks[0].ColumnName
		}
		return fks[0].ReferencedColumn
	}
	names := make([]string, len(fks))
	for i, fk := range fks {
		if local {
			names[i] = fk.ColumnName
		} else {
			names[i] = fk.ReferencedColumn
		}
	}
	return "(" + strings.Join(names, ",") + ")"
}

func edgeColNames(edges []fkEdge, parent bool) string {
	if len(edges) == 1 {
		if parent {
			return edges[0].ParentColumn
		}
		return edges[0].ChildColumn
	}
	names := make([]string, len(edges))
	for i, edge := range edges {
		if parent {
			names[i] = edge.ParentColumn
		} else {
			names[i] = edge.ChildColumn
		}
	}
	return "(" + strings.Join(names, ",") + ")"
}
