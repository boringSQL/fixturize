package fixturize

import (
	"database/sql"
	"fmt"
	"strings"
)

type (
	DatabaseSchema struct {
		tables map[string]*TableInfo
	}

	TableInfo struct {
		Schema      string
		Name        string
		Columns     map[string]*ColumnInfo
		PrimaryKey  []string
		ForeignKeys []*ForeignKeyInfo
	}

	ColumnInfo struct {
		Name         string
		Type         string
		IsNullable   bool
		IsPrimaryKey bool
		IsForeignKey bool
		IsUnique     bool
		ForeignKey   *ForeignKeyInfo
		Default      *string
		MaxLength    *int
	}

	ForeignKeyInfo struct {
		ConstraintName   string
		ColumnName       string
		ReferencedTable  string
		ReferencedColumn string
	}
)

func IntrospectSchema(db *sql.DB) (*DatabaseSchema, error) {
	dbSchema := &DatabaseSchema{
		tables: make(map[string]*TableInfo),
	}

	tables, err := getTables(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}

	partitionMap, err := getPartitionParents(db)
	if err != nil {
		return nil, fmt.Errorf("failed to get partition map: %w", err)
	}

	for _, qualifiedName := range tables {
		schemaName, tableName := parseTableName(qualifiedName)

		tableInfo := &TableInfo{
			Schema:  schemaName,
			Name:    tableName,
			Columns: make(map[string]*ColumnInfo),
		}

		columns, err := getColumns(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns for table '%s': %w", qualifiedName, err)
		}
		tableInfo.Columns = columns

		primaryKeys, err := getPrimaryKeys(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get primary keys for table '%s': %w", qualifiedName, err)
		}
		tableInfo.PrimaryKey = primaryKeys

		for _, pkCol := range primaryKeys {
			if col, exists := tableInfo.Columns[pkCol]; exists {
				col.IsPrimaryKey = true
			}
		}

		foreignKeys, err := getForeignKeys(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get foreign keys for table '%s': %w", qualifiedName, err)
		}

		// Make sure FK references to partitions back to the parent table
		for _, fk := range foreignKeys {
			if parent, ok := partitionMap[fk.ReferencedTable]; ok {
				fk.ReferencedTable = parent
			}
		}

		tableInfo.ForeignKeys = foreignKeys

		for _, fk := range foreignKeys {
			if col, exists := tableInfo.Columns[fk.ColumnName]; exists {
				col.IsForeignKey = true
				col.ForeignKey = fk
			}
		}

		uniqueCols, err := getUniqueColumns(db, schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get unique constraints for table '%s': %w", qualifiedName, err)
		}
		for colName := range uniqueCols {
			if col, exists := tableInfo.Columns[colName]; exists {
				col.IsUnique = true
			}
		}

		dbSchema.tables[qualifiedName] = tableInfo
	}

	return dbSchema, nil
}

func getTables(db *sql.DB) ([]string, error) {
	query := `
		SELECT table_schema, table_name
		FROM information_schema.tables t
		WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
		  AND table_type IN ('BASE TABLE', 'PARTITIONED TABLE')
		  AND NOT EXISTS (
		      SELECT 1 FROM pg_inherits
		      JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
		      WHERE pg_inherits.inhrelid = (quote_ident(t.table_schema) || '.' || quote_ident(t.table_name))::regclass
		        AND parent.relkind = 'p'
		  )
		ORDER BY table_schema, table_name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var schemaName, tableName string
		if err := rows.Scan(&schemaName, &tableName); err != nil {
			return nil, err
		}
		tables = append(tables, schemaName+"."+tableName)
	}

	return tables, rows.Err()
}

func getPartitionParents(db *sql.DB) (map[string]string, error) {
	query := `
		WITH RECURSIVE partition_tree AS (
			-- base: direct children of root partitioned tables
			SELECT
				child.oid AS partition_oid,
				parent.oid AS root_oid
			FROM pg_inherits
			JOIN pg_class child ON pg_inherits.inhrelid = child.oid
			JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
			WHERE parent.relkind = 'p'
			  AND NOT EXISTS (
			      SELECT 1 FROM pg_inherits pi2
			      JOIN pg_class gp ON pi2.inhparent = gp.oid
			      WHERE pi2.inhrelid = parent.oid AND gp.relkind = 'p'
			  )

			UNION ALL

			-- recurse: sub-partitions point to the same root
			SELECT
				child.oid,
				pt.root_oid
			FROM pg_inherits
			JOIN pg_class child ON pg_inherits.inhrelid = child.oid
			JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
			JOIN partition_tree pt ON pt.partition_oid = parent.oid
			WHERE parent.relkind = 'p'
		)
		SELECT
			pns.nspname || '.' || pc.relname,
			rns.nspname || '.' || rc.relname
		FROM partition_tree
		JOIN pg_class pc ON partition_tree.partition_oid = pc.oid
		JOIN pg_namespace pns ON pc.relnamespace = pns.oid
		JOIN pg_class rc ON partition_tree.root_oid = rc.oid
		JOIN pg_namespace rns ON rc.relnamespace = rns.oid
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var partition, root string
		if err := rows.Scan(&partition, &root); err != nil {
			return nil, err
		}
		m[partition] = root
	}

	return m, rows.Err()
}

func parseTableName(name string) (schema, table string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "public", name
}

func getColumns(db *sql.DB, schemaName, tableName string) (map[string]*ColumnInfo, error) {
	query := `
		SELECT
			column_name,
			data_type,
			is_nullable,
			column_default,
			character_maximum_length
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = $2
		ORDER BY ordinal_position
	`

	rows, err := db.Query(query, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]*ColumnInfo)
	for rows.Next() {
		var (
			columnName    string
			dataType      string
			isNullable    string
			columnDefault *string
			maxLength     *int64
		)

		if err := rows.Scan(&columnName, &dataType, &isNullable, &columnDefault, &maxLength); err != nil {
			return nil, err
		}

		col := &ColumnInfo{
			Name:       columnName,
			Type:       dataType,
			IsNullable: isNullable == "YES",
			Default:    columnDefault,
		}

		if maxLength != nil {
			length := int(*maxLength)
			col.MaxLength = &length
		}

		columns[columnName] = col
	}

	return columns, rows.Err()
}

func getPrimaryKeys(db *sql.DB, schemaName, tableName string) ([]string, error) {
	qualifiedName := schemaName + "." + tableName
	query := `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass
		  AND i.indisprimary
		ORDER BY array_position(i.indkey, a.attnum)
	`

	rows, err := db.Query(query, qualifiedName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var primaryKeys []string
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil {
			return nil, err
		}
		primaryKeys = append(primaryKeys, columnName)
	}

	return primaryKeys, rows.Err()
}

func getForeignKeys(db *sql.DB, schemaName, tableName string) ([]*ForeignKeyInfo, error) {
	query := `
		SELECT
			c.conname,
			a_local.attname,
			ns_ref.nspname,
			cl_ref.relname,
			a_ref.attname
		FROM pg_constraint c
		JOIN pg_class cl ON c.conrelid = cl.oid
		JOIN pg_namespace ns ON cl.relnamespace = ns.oid
		JOIN pg_class cl_ref ON c.confrelid = cl_ref.oid
		JOIN pg_namespace ns_ref ON cl_ref.relnamespace = ns_ref.oid
		CROSS JOIN LATERAL unnest(c.conkey, c.confkey)
			WITH ORDINALITY AS cols(local_col, ref_col, ord)
		JOIN pg_attribute a_local
			ON a_local.attrelid = c.conrelid AND a_local.attnum = cols.local_col
		JOIN pg_attribute a_ref
			ON a_ref.attrelid = c.confrelid AND a_ref.attnum = cols.ref_col
		WHERE c.contype = 'f'
		  AND ns.nspname = $1
		  AND cl.relname = $2
		  AND NOT EXISTS (
		      SELECT 1 FROM pg_inherits
		      JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
		      WHERE pg_inherits.inhrelid = c.confrelid
		        AND parent.relkind = 'p'
		  )
		ORDER BY c.conname, cols.ord
	`

	rows, err := db.Query(query, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foreignKeys []*ForeignKeyInfo
	for rows.Next() {
		var fk ForeignKeyInfo
		var refSchema string
		if err := rows.Scan(&fk.ConstraintName, &fk.ColumnName, &refSchema, &fk.ReferencedTable, &fk.ReferencedColumn); err != nil {
			return nil, err
		}
		fk.ReferencedTable = refSchema + "." + fk.ReferencedTable
		foreignKeys = append(foreignKeys, &fk)
	}

	return foreignKeys, rows.Err()
}

func getUniqueColumns(db *sql.DB, schemaName, tableName string) (map[string]bool, error) {
	qualifiedName := schemaName + "." + tableName
	query := `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass
		  AND i.indisunique
		  AND NOT i.indisprimary
		  AND array_length(i.indkey, 1) = 1
	`

	rows, err := db.Query(query, qualifiedName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	uniqueCols := make(map[string]bool)
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, err
		}
		uniqueCols[colName] = true
	}
	return uniqueCols, rows.Err()
}

func (ds *DatabaseSchema) GetTable(name string) (*TableInfo, error) {
	if table, exists := ds.tables[name]; exists {
		return table, nil
	}
	if !strings.Contains(name, ".") {
		if table, exists := ds.tables["public."+name]; exists {
			return table, nil
		}
	}
	return nil, fmt.Errorf("table not found: %s", name)
}

func (ds *DatabaseSchema) GetTables() []string {
	tables := make([]string, 0, len(ds.tables))
	for name := range ds.tables {
		tables = append(tables, name)
	}
	return tables
}

func (ds *DatabaseSchema) HasTable(name string) bool {
	if _, exists := ds.tables[name]; exists {
		return true
	}
	if !strings.Contains(name, ".") {
		_, exists := ds.tables["public."+name]
		return exists
	}
	return false
}

// TopologicalSort orders tables by FK dependencies using Kahn's algorithm.
// Returns tables in insertion order (parents before children).
// Tables involved in cycles are appended at the end with a warning via onCycle callback.
func (ds *DatabaseSchema) TopologicalSort(tables []string, onCycle func(table string)) []string {
	tableSet := make(map[string]bool, len(tables))
	for _, t := range tables {
		tableSet[t] = true
	}

	deps := make(map[string]map[string]bool)
	for _, tableName := range tables {
		deps[tableName] = make(map[string]bool)
		tableInfo, err := ds.GetTable(tableName)
		if err != nil {
			continue
		}
		qualifiedName := tableInfo.Schema + "." + tableInfo.Name
		for _, fk := range tableInfo.ForeignKeys {
			parent := fk.ReferencedTable
			if parent == qualifiedName {
				continue
			}
			if tableSet[parent] {
				deps[tableName][parent] = true
			}
		}
	}

	inDegree := make(map[string]int)
	for _, t := range tables {
		inDegree[t] = len(deps[t])
	}

	var queue []string
	for t, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, t)
		}
	}

	var result []string
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		result = append(result, t)

		for other, d := range deps {
			if d[t] {
				delete(d, t)
				inDegree[other]--
				if inDegree[other] == 0 {
					queue = append(queue, other)
				}
			}
		}
	}

	for _, t := range tables {
		found := false
		for _, r := range result {
			if r == t {
				found = true
				break
			}
		}
		if !found {
			result = append(result, t)
			if onCycle != nil {
				onCycle(t)
			}
		}
	}

	return result
}
