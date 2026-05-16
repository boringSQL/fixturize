package fixturize

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (e *Extractor) loadGeneratedColumns() error {
	query := `
		SELECT table_schema || '.' || table_name, column_name,
		       is_generated, identity_generation
		FROM information_schema.columns
		WHERE (is_generated = 'ALWAYS' OR identity_generation = 'ALWAYS')
		  AND table_schema NOT IN ('pg_catalog', 'information_schema')
	`

	rows, err := e.tx.Query(e.ctx, query)
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

func (e *Extractor) collectRows(tableName string, rows pgx.Rows) error {
	fds := rows.FieldDescriptions()

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
		vals, err := rows.Values()
		if err != nil {
			return fmt.Errorf("scan failed for %s: %w", tableName, err)
		}

		row := make(map[string]any, len(fds))
		for i, fd := range fds {
			col := fd.Name
			if genCols[col] {
				continue
			}
			row[col] = convertValue(vals[i], fd.DataTypeOID)
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

func convertValue(val any, oid uint32) any {
	if val == nil {
		return nil
	}

	switch oid {
	case pgtype.DateOID:
		if t, ok := val.(time.Time); ok {
			return t.Format("2006-01-02")
		}
	case pgtype.TimestampOID, pgtype.TimestamptzOID:
		if t, ok := val.(time.Time); ok {
			return t.Format(time.RFC3339)
		}
	case pgtype.JSONOID, pgtype.JSONBOID:
		// pgx decodes JSON/JSONB into Go values (map/slice/etc.) by default,
		// not raw bytes — so we have to re-marshal to get a valid JSON string.
		// String/[]byte branches cover odd codec registrations.
		switch v := val.(type) {
		case []byte:
			return string(v)
		case string:
			return v
		default:
			if b, err := json.Marshal(v); err == nil {
				return string(b)
			}
		}
	case pgtype.UUIDOID:
		switch v := val.(type) {
		case [16]byte:
			return formatUUID(v[:])
		case []byte:
			return formatUUID(v)
		}
	case pgtype.ByteaOID:
		if b, ok := val.([]byte); ok {
			return `\x` + hex.EncodeToString(b)
		}
	case pgtype.NumericOID:
		// pgx returns pgtype.Numeric (struct) here; %v would print struct fields.
		// Value() yields the canonical decimal string with full precision.
		if n, ok := val.(pgtype.Numeric); ok {
			if v, err := n.Value(); err == nil && v != nil {
				return v
			}
		}
	}

	switch v := val.(type) {
	case time.Time:
		return v.Format(time.RFC3339)
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
