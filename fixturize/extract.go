package fixturize

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type (
	ExtractOptions struct {
		Connection       string
		Root             string
		Schema           string
		Output           string
		Limit            int
		Depth            int
		Include          []string
		Exclude          []string
		Mask             []string
		StatementTimeout int
		DryRun           bool
	}

	ExtractResult struct {
		Fixture  *Fixture
		JSON     []byte
		Tables   []ExtractedTable
		Warnings []string
	}

	ExtractedTable struct {
		Name     string
		RowCount int
	}

	Extractor struct {
		db             *sql.DB
		tx             *sql.Tx
		schema         *DatabaseSchema
		options        *ExtractOptions
		invertedGraph  map[string][]fkEdge
		collected      map[string][]map[string]any
		collectedPKs   map[string]map[any]bool
		excludeSet      map[string]bool
		generatedCols   map[string]map[string]bool
		identityTables  map[string]bool
		masks           map[string]map[string]string
		columnOrders    map[string][]string
		warnings        []string
		warnedParentFKs map[string]bool
	}

	fkEdge struct {
		ChildTable   string
		ChildColumn  string
		ParentColumn string
	}
)

const maxTraversalIterations = 100

func NewExtractor(db *sql.DB, options *ExtractOptions) *Extractor {
	excludeSet := make(map[string]bool)
	for _, t := range options.Exclude {
		excludeSet[t] = true
	}

	return &Extractor{
		db:              db,
		options:         options,
		invertedGraph:   make(map[string][]fkEdge),
		collected:       make(map[string][]map[string]any),
		collectedPKs:    make(map[string]map[any]bool),
		excludeSet:      excludeSet,
		generatedCols:   make(map[string]map[string]bool),
		identityTables:  make(map[string]bool),
		masks:           make(map[string]map[string]string),
		columnOrders:    make(map[string][]string),
		warnedParentFKs: make(map[string]bool),
	}
}

func (e *Extractor) Extract() (*ExtractResult, error) {
	fmt.Print("Introspecting schema... ")
	schema, err := IntrospectSchema(e.db)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect schema: %w", err)
	}
	e.schema = schema
	fmt.Printf("%d table(s)\n", len(schema.GetTables()))

	if err := e.parseMasks(); err != nil {
		return nil, err
	}

	tx, err := e.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	e.tx = tx
	defer tx.Rollback()

	if _, err := tx.Exec("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ"); err != nil {
		return nil, fmt.Errorf("failed to set isolation level: %w", err)
	}

	timeout := e.options.StatementTimeout
	if timeout <= 0 {
		timeout = 30
	}
	if _, err := tx.Exec(fmt.Sprintf("SET statement_timeout = '%ds'", timeout)); err != nil {
		return nil, fmt.Errorf("failed to set statement_timeout: %w", err)
	}

	if err := e.loadGeneratedColumns(); err != nil {
		return nil, fmt.Errorf("failed to load generated columns: %w", err)
	}

	e.buildInvertedGraph()

	rootTable, clause, err := e.parseRootSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to parse root spec: %w", err)
	}

	rootCols, err := e.selectColumns(rootTable)
	if err != nil {
		return nil, fmt.Errorf("failed to build column list for %s: %w", rootTable, err)
	}
	rootQuery := fmt.Sprintf("SELECT %s FROM %s", rootCols, QuoteQualifiedTable(rootTable))
	if clause != "" {
		rootQuery += " " + clause
	}
	fmt.Printf("Root query: %s\n", rootQuery)

	fmt.Print("Validating query... ")
	if _, err := e.tx.Exec("EXPLAIN " + rootQuery); err != nil {
		fmt.Println("FAILED")
		return nil, fmt.Errorf("invalid root query:\n  %s\n  %w", rootQuery, err)
	}
	fmt.Println("OK")

	fmt.Printf("Querying %s... ", shortName(rootTable))
	if err := e.extractRootRows(rootTable, clause); err != nil {
		return nil, fmt.Errorf("root query failed:\n  %s\n  %w", rootQuery, err)
	}

	rootCount := len(e.collected[rootTable])
	if rootCount == 0 {
		fmt.Println("0 rows")
		return nil, fmt.Errorf("no rows matched root query:\n  %s", rootQuery)
	}
	fmt.Printf("%d row(s)\n", rootCount)

	for _, incl := range e.options.Include {
		tableName, err := e.resolveTableName(incl)
		if err != nil {
			return nil, fmt.Errorf("--include table %q: %w", incl, err)
		}
		if err := e.extractAllRows(tableName); err != nil {
			return nil, fmt.Errorf("failed to extract included table %q: %w", incl, err)
		}
		fmt.Printf("  + %s... %d row(s) (included)\n", shortName(tableName), len(e.collected[tableName]))
	}

	for iteration := 0; iteration < maxTraversalIterations; iteration++ {
		newParents, err := e.extractParents()
		if err != nil {
			return nil, fmt.Errorf("failed to extract parents: %w", err)
		}

		newChildren, err := e.extractChildren(rootTable)
		if err != nil {
			return nil, fmt.Errorf("failed to extract children: %w", err)
		}

		if !newParents && !newChildren {
			break
		}
	}

	orderedTables, err := e.orderTablesForOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to order tables: %w", err)
	}

	fixture := e.buildFixture(orderedTables)
	jsonData, err := fixture.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to generate JSON: %w", err)
	}

	var tables []ExtractedTable
	totalRows := 0
	for _, t := range orderedTables {
		rows := e.collected[t]
		tables = append(tables, ExtractedTable{Name: t, RowCount: len(rows)})
		totalRows += len(rows)
	}

	fmt.Printf("Extracted %d table(s), %d row(s) total\n", len(orderedTables), totalRows)

	return &ExtractResult{
		Fixture:  fixture,
		JSON:     jsonData,
		Tables:   tables,
		Warnings: e.warnings,
	}, nil
}

func (e *Extractor) parseRootSpec() (table, clause string, err error) {
	spec := strings.TrimSpace(e.options.Root)
	if spec == "" {
		return "", "", fmt.Errorf("--root is required")
	}

	parts := strings.SplitN(spec, " ", 2)
	rawTable := parts[0]

	tableName, err := e.resolveTableName(rawTable)
	if err != nil {
		return "", "", fmt.Errorf("root table %q: %w", rawTable, err)
	}

	if len(parts) == 2 {
		clause = parts[1]
	}

	return tableName, clause, nil
}

func (e *Extractor) resolveTableName(name string) (string, error) {
	if strings.Contains(name, ".") {
		if e.schema.HasTable(name) {
			return name, nil
		}
		return "", fmt.Errorf("table not found: %s", name)
	}

	qualified := e.options.Schema + "." + name
	if e.schema.HasTable(qualified) {
		return qualified, nil
	}

	return "", fmt.Errorf("table not found: %s (tried %s)", name, qualified)
}

func (e *Extractor) parseMasks() error {
	for _, spec := range e.options.Mask {
		eqIdx := strings.Index(spec, "=")
		if eqIdx == -1 {
			return fmt.Errorf("invalid --mask format %q, expected table.column=expression", spec)
		}
		left := spec[:eqIdx]
		expr := spec[eqIdx+1:]
		if expr == "" {
			return fmt.Errorf("invalid --mask format %q, expression cannot be empty", spec)
		}

		lastDot := strings.LastIndex(left, ".")
		if lastDot == -1 {
			return fmt.Errorf("invalid --mask format %q, expected table.column=expression", spec)
		}
		rawTable := left[:lastDot]
		column := left[lastDot+1:]

		tableName, err := e.resolveTableName(rawTable)
		if err != nil {
			return fmt.Errorf("--mask table %q: %w", rawTable, err)
		}

		tableInfo, _ := e.schema.GetTable(tableName)
		if tableInfo != nil {
			if _, ok := tableInfo.Columns[column]; !ok {
				return fmt.Errorf("--mask column %q not found in table %s", column, tableName)
			}
		}

		if e.masks[tableName] == nil {
			e.masks[tableName] = make(map[string]string)
		}
		e.masks[tableName][column] = expr
		fmt.Printf("Mask: %s.%s = %s\n", shortName(tableName), column, expr)
	}
	return nil
}

func (e *Extractor) selectColumns(tableName string) (string, error) {
	tableMasks := e.masks[tableName]
	genCols := e.generatedCols[tableName]

	if len(tableMasks) == 0 && len(genCols) == 0 {
		return "*", nil
	}

	cols, err := e.getOrderedColumns(tableName)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, col := range cols {
		if genCols[col] {
			continue
		}
		if expr, masked := tableMasks[col]; masked {
			parts = append(parts, fmt.Sprintf("(%s) AS %s", expr, QuoteIdent(col)))
		} else {
			parts = append(parts, QuoteIdent(col))
		}
	}

	return strings.Join(parts, ", "), nil
}

func (e *Extractor) getOrderedColumns(tableName string) ([]string, error) {
	if cols, ok := e.columnOrders[tableName]; ok {
		return cols, nil
	}

	schemaName, table := parseTableName(tableName)
	query := `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`

	rows, err := e.tx.Query(query, schemaName, table)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns for %s: %w", tableName, err)
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	e.columnOrders[tableName] = cols
	return cols, nil
}

func (e *Extractor) buildInvertedGraph() {
	for _, tableName := range e.schema.GetTables() {
		tableInfo, _ := e.schema.GetTable(tableName)
		for _, fk := range tableInfo.ForeignKeys {
			parentTable := fk.ReferencedTable
			e.invertedGraph[parentTable] = append(e.invertedGraph[parentTable], fkEdge{
				ChildTable:   tableName,
				ChildColumn:  fk.ColumnName,
				ParentColumn: fk.ReferencedColumn,
			})
		}
	}
}

func (e *Extractor) loadGeneratedColumns() error {
	query := `
		SELECT table_schema || '.' || table_name, column_name, identity_generation
		FROM information_schema.columns
		WHERE (is_generated = 'ALWAYS' OR identity_generation = 'ALWAYS')
		  AND table_schema NOT IN ('pg_catalog', 'information_schema')
	`

	rows, err := e.tx.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, colName string
		var identityGen *string
		if err := rows.Scan(&tableName, &colName, &identityGen); err != nil {
			return err
		}

		if _, ok := e.generatedCols[tableName]; !ok {
			e.generatedCols[tableName] = make(map[string]bool)
		}
		e.generatedCols[tableName][colName] = true

		if identityGen != nil && *identityGen == "ALWAYS" {
			e.identityTables[tableName] = true
		}
	}

	return rows.Err()
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

		for _, fk := range tableInfo.ForeignKeys {
			parentTable := fk.ReferencedTable

			if e.excludeSet[shortName(parentTable)] || e.excludeSet[parentTable] {
				e.warnExcludedParent(parentTable, tableName)
				continue
			}

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

			parentCols, err := e.selectColumns(parentTable)
			if err != nil {
				return false, err
			}
			query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ANY($1)",
				parentCols, QuoteQualifiedTable(parentTable), QuoteIdent(fk.ReferencedColumn))

			rows, err := e.tx.Query(query, fkValues)
			if err != nil {
				return false, fmt.Errorf("parent query for %s failed: %w", parentTable, err)
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
				fmt.Printf("  ↑ %s... %d row(s) (parent of %s)\n", shortName(parentTable), newRows, shortName(tableName))
			}
		}
	}

	return foundNew, nil
}

func (e *Extractor) extractChildren(rootTable string) (bool, error) {
	foundNew := false

	type bfsItem struct {
		table string
		depth int
	}

	visited := make(map[string]bool)
	queue := []bfsItem{{table: rootTable, depth: 0}}
	visited[rootTable] = true

	for t := range e.collected {
		if !visited[t] {
			visited[t] = true
			queue = append(queue, bfsItem{table: t, depth: 0})
		}
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

		edges := e.invertedGraph[item.table]
		for _, edge := range edges {
			childTable := edge.ChildTable

			if e.excludeSet[shortName(childTable)] || e.excludeSet[childTable] {
				continue
			}

			parentRows := e.collected[item.table]
			if len(parentRows) == 0 {
				continue
			}

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

			childCols, err := e.selectColumns(childTable)
			if err != nil {
				return false, err
			}
			query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ANY($1) AND %s IS NOT NULL",
				childCols, QuoteQualifiedTable(childTable), QuoteIdent(edge.ChildColumn), QuoteIdent(edge.ChildColumn))

			if e.options.Limit > 0 {
				query += fmt.Sprintf(" LIMIT %d", e.options.Limit)
			}

			rows, err := e.tx.Query(query, pkValues)
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
				e.warn("table %s limited to %d rows, subgraph may be incomplete", shortName(childTable), e.options.Limit)
			}

			if afterCount > beforeCount {
				foundNew = true
				newRows := afterCount - beforeCount
				fmt.Printf("  ↓ %s... %d row(s)\n", shortName(childTable), newRows)
			}

			if err := e.extractSelfReferencing(childTable); err != nil {
				return false, err
			}

			if !visited[childTable] {
				visited[childTable] = true
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

func (e *Extractor) collectRows(tableName string, rows *sql.Rows) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return err
	}

	tableInfo, _ := e.schema.GetTable(tableName)
	var pkCols []string
	if tableInfo != nil {
		pkCols = tableInfo.PrimaryKey
	}

	genCols := e.generatedCols[tableName]

	if e.collectedPKs[tableName] == nil {
		e.collectedPKs[tableName] = make(map[any]bool)
	}

	for rows.Next() {
		dest := make([]any, len(columns))
		for i := range dest {
			dest[i] = new(any)
		}

		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("scan failed for %s: %w", tableName, err)
		}

		row := make(map[string]any, len(columns))
		for i, col := range columns {
			if genCols[col] {
				continue
			}

			rawVal := *(dest[i].(*any))
			row[col] = convertValue(rawVal, colTypes[i])
		}

		if len(pkCols) > 0 {
			pkKey := buildPKKey(row, pkCols)
			if e.collectedPKs[tableName][pkKey] {
				continue
			}
			e.collectedPKs[tableName][pkKey] = true
		}

		e.collected[tableName] = append(e.collected[tableName], row)
	}

	return rows.Err()
}

func convertValue(val any, colType *sql.ColumnType) any {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case time.Time:
		typeName := strings.ToLower(colType.DatabaseTypeName())
		if typeName == "DATE" || typeName == "date" {
			return v.Format("2006-01-02")
		}
		return v.Format(time.RFC3339)

	case []byte:
		typeName := strings.ToUpper(colType.DatabaseTypeName())
		if typeName == "JSON" || typeName == "JSONB" {
			return string(v)
		}
		if typeName == "UUID" || len(v) == 16 {
			return formatUUID(v)
		}
		return `\x` + hex.EncodeToString(v)

	case [16]byte:
		return formatUUID(v[:])

	case int64, int32, int16, int8, float64, float32, bool, string:
		return v

	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatUUID(b []byte) string {
	if len(b) != 16 {
		return hex.EncodeToString(b)
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func buildPKKey(row map[string]any, pkCols []string) any {
	if len(pkCols) == 1 {
		return row[pkCols[0]]
	}
	parts := make([]string, len(pkCols))
	for i, col := range pkCols {
		parts[i] = fmt.Sprintf("%v", row[col])
	}
	return strings.Join(parts, "|")
}

func (e *Extractor) orderTablesForOutput() ([]string, error) {
	tables := make([]string, 0, len(e.collected))
	for t := range e.collected {
		tables = append(tables, t)
	}
	sort.Strings(tables)

	result := e.schema.TopologicalSort(tables, func(t string) {
		e.warn("circular FK dependency involving %s; insertion order may need manual adjustment", shortName(t))
	})

	return result, nil
}

func (e *Extractor) buildFixture(orderedTables []string) *Fixture {
	var appliedMasks []string
	for _, m := range e.options.Mask {
		appliedMasks = append(appliedMasks, m)
	}

	fixture := NewFixture(e.options.Root, appliedMasks)

	for _, tableName := range orderedTables {
		rows := e.collected[tableName]
		if len(rows) == 0 {
			continue
		}

		cols, _ := e.getOrderedColumns(tableName)
		genCols := e.generatedCols[tableName]

		var filteredCols []string
		for _, col := range cols {
			if !genCols[col] {
				filteredCols = append(filteredCols, col)
			}
		}

		var rowData [][]any
		for _, row := range rows {
			var rowArr []any
			for _, col := range filteredCols {
				rowArr = append(rowArr, row[col])
			}
			rowData = append(rowData, rowArr)
		}

		fixture.AddTable(tableName, filteredCols, rowData)
	}

	// Compute untouched tables
	allTables := e.schema.GetTables()
	collectedSet := make(map[string]bool)
	for t := range e.collected {
		collectedSet[t] = true
	}

	var untouched []string
	for _, t := range allTables {
		if !collectedSet[t] && !e.excludeSet[shortName(t)] && !e.excludeSet[t] {
			untouched = append(untouched, shortName(t))
		}
	}
	sort.Strings(untouched)
	fixture.SetUntouchedTables(untouched)

	return fixture
}

func (e *Extractor) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	e.warnings = append(e.warnings, msg)
	fmt.Printf("  Warning: %s\n", msg)
}

func (e *Extractor) warnExcludedParent(parentTable, childTable string) {
	key := parentTable + "->" + childTable
	if e.warnedParentFKs[key] {
		return
	}
	e.warnedParentFKs[key] = true
	e.warn("excluded table %s is referenced by %s; fixture may have dangling FKs (%s)",
		shortName(parentTable), shortName(childTable), key)
}

func shortName(qualifiedName string) string {
	_, table := parseTableName(qualifiedName)
	return table
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
