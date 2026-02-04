package fixturize

import "strings"

func QuoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func QuoteQualifiedTable(qualifiedName string) string {
	schema, table := parseTableName(qualifiedName)
	return QuoteIdent(schema) + "." + QuoteIdent(table)
}

func QuoteColumns(cols []string) string {
	quoted := make([]string, len(cols))
	for i, col := range cols {
		quoted[i] = QuoteIdent(col)
	}
	return strings.Join(quoted, ", ")
}
