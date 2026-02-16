package fixturize

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
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
		Filter           []string
		StatementTimeout int
		DryRun           bool
		Verbose          bool
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
		db              *sql.DB
		tx              *sql.Tx
		schema          *DatabaseSchema
		options         *ExtractOptions
		invertedGraph   map[string][]fkEdge
		collected       map[string][]map[string]any
		collectedPKs    map[string]map[any]bool
		excludeSet      map[string]bool
		generatedCols   map[string]map[string]bool
		identityTables  map[string]bool
		masks           map[string]map[string]string
		filters         map[string]string
		columnOrders    map[string][]string
		warnings        []string
		warnedParentFKs map[string]bool
		downwardTables  map[string]bool // tables we found by going downward (root + children)
		limitReached    map[string]bool // tables where we already hit --limit so they are not queried again
	}

	fkEdge struct {
		ConstraintName string
		ChildTable     string
		ChildColumn    string
		ParentColumn   string
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
		filters:         make(map[string]string),
		columnOrders:    make(map[string][]string),
		warnedParentFKs: make(map[string]bool),
		downwardTables:  make(map[string]bool),
		limitReached:    make(map[string]bool),
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
	if err := e.parseFilters(); err != nil {
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
	e.downwardTables[rootTable] = true

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
		e.downwardTables[tableName] = true
		fmt.Printf("  + %s... %d row(s) (included)\n", shortName(tableName), len(e.collected[tableName]))
	}

	// 1st pass: walk all child edges from the root; this discovers
	// the full subgraph of data that "belongs" to the root entity.
	if _, err := e.extractChildren(); err != nil {
		return nil, fmt.Errorf("failed to extract children: %w", err)
	}

	// 2nd pass: fetch any parent rows needed to maintain FK integrity.
	for iteration := 0; iteration < maxTraversalIterations; iteration++ {
		newParents, err := e.extractParents()
		if err != nil {
			return nil, fmt.Errorf("failed to extract parents: %w", err)
		}
		if !newParents {
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

func (e *Extractor) parseFilters() error {
	for _, spec := range e.options.Filter {
		eqIdx := strings.Index(spec, "=")
		if eqIdx == -1 {
			return fmt.Errorf("invalid --filter format %q, expected table=expression", spec)
		}
		rawTable := spec[:eqIdx]
		expr := spec[eqIdx+1:]
		if expr == "" {
			return fmt.Errorf("invalid --filter format %q, expression cannot be empty", spec)
		}

		tableName, err := e.resolveTableName(rawTable)
		if err != nil {
			return fmt.Errorf("--filter table %q: %w", rawTable, err)
		}

		e.filters[tableName] = expr
		fmt.Printf("Filter: %s = %s\n", shortName(tableName), expr)
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

	var appliedFilters []string
	for _, f := range e.options.Filter {
		appliedFilters = append(appliedFilters, f)
	}

	fixture := NewFixture(e.options.Root, appliedMasks, appliedFilters)
	fixture.TableOrder = orderedTables

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
		if e.identityTables[tableName] {
			fixture.Tables[tableName].IdentityColumns = true
		}
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
			untouched = append(untouched, t)
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
