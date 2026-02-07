package fixturize

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func (e *Extractor) loadGeneratedColumns() error {
	query := `
		SELECT table_schema || '.' || table_name, column_name,
		       is_generated, identity_generation
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
		var tableName, colName, isGenerated string
		var identityGen *string
		if err := rows.Scan(&tableName, &colName, &isGenerated, &identityGen); err != nil {
			return err
		}

		// there's a problem with computed columns and how they need to be treated
		// - computed colums (GENERATED ALWAYS AS expr STORED) are exlcuded from
		//   the extraction.
		// - identity columns (GENERATED ... AS IDENTITY) are kept to maintain the
		//   referential integrity; apply side uses OVERRIDE SYTEM VALUE
		if isGenerated == "ALWAYS" {
			if _, ok := e.generatedCols[tableName]; !ok {
				e.generatedCols[tableName] = make(map[string]bool)
			}
			e.generatedCols[tableName][colName] = true
		}

		// track identity tables
		if identityGen != nil && *identityGen == "ALWAYS" {
			e.identityTables[tableName] = true
		}
	}

	return rows.Err()
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
