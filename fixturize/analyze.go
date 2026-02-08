package fixturize

import (
	"fmt"
	"strings"
)

type (
	ConfidenceLevel int

	PIIMatch struct {
		Table      string
		Column     string
		ColumnType string
		Category   string
		Confidence ConfidenceLevel
		MaskExpr   string
	}

	AnalyzeResult struct {
		Matches       map[string][]PIIMatch // table name → matches
		TablesScanned int
	}
)

const (
	ConfidenceLow    ConfidenceLevel = 30
	ConfidenceMedium ConfidenceLevel = 60
	ConfidenceHigh   ConfidenceLevel = 90
)

func (c ConfidenceLevel) String() string {
	switch {
	case c >= ConfidenceHigh:
		return "HIGH"
	case c >= ConfidenceMedium:
		return "MED"
	default:
		return "LOW"
	}
}

func ParseConfidence(s string) (ConfidenceLevel, error) {
	switch strings.ToLower(s) {
	case "low":
		return ConfidenceLow, nil
	case "medium", "med":
		return ConfidenceMedium, nil
	case "high":
		return ConfidenceHigh, nil
	default:
		return 0, fmt.Errorf("invalid confidence level %q (use low, medium, high)", s)
	}
}

func AnalyzeSchema(schema *DatabaseSchema, tables []string, minConfidence ConfidenceLevel) *AnalyzeResult {
	result := &AnalyzeResult{
		Matches:       make(map[string][]PIIMatch),
		TablesScanned: len(tables),
	}

	for _, tableName := range tables {
		table, err := schema.GetTable(tableName)
		if err != nil {
			continue
		}

		pkCols := table.PrimaryKey
		pkIsInt := pkColumnsAreInteger(table)

		for _, col := range sortedColumns(table) {
			if col.IsPrimaryKey || col.IsForeignKey {
				continue
			}

			match, ok := matchColumn(col, pkCols, pkIsInt)
			if !ok || match.Confidence < minConfidence {
				continue
			}

			match.Table = tableName
			result.Matches[tableName] = append(result.Matches[tableName], match)
		}
	}

	return result
}

func matchColumn(col *ColumnInfo, pkCols []string, pkIsInt bool) (PIIMatch, bool) {
	var best PIIMatch
	bestScore := ConfidenceLevel(-1)

	for _, rule := range piiRules {
		nameMatch := len(rule.Names) > 0 && matchesColumnName(col.Name, rule.Names)
		typeMatch := len(rule.Types) > 0 && matchesType(col.Type, rule.Types)
		typeOnly := len(rule.Names) == 0 && typeMatch

		if !nameMatch && !typeOnly {
			continue
		}

		// Skip integer columns matched by name only (likely IDs, not PII)
		if nameMatch && !typeMatch && isIntegerType(col.Type) {
			continue
		}

		var confidence ConfidenceLevel
		switch {
		case nameMatch && typeMatch:
			confidence = ConfidenceMedium
		case typeOnly:
			confidence = ConfidenceMedium
		case nameMatch:
			confidence = ConfidenceLow
		}

		if confidence > bestScore {
			bestScore = confidence
			best = PIIMatch{
				Column:     col.Name,
				ColumnType: col.Type,
				Category:   rule.Category,
				Confidence: confidence,
				MaskExpr:   resolveMask(rule.Mask, pkCols, pkIsInt),
			}
		}
	}

	return best, bestScore >= ConfidenceLow
}

func resolveMask(template string, pkCols []string, pkIsInt bool) string {
	if len(pkCols) == 0 {
		s := strings.ReplaceAll(template, "{pk}", "'0'")
		s = strings.ReplaceAll(s, "{pki}", "0")
		return s
	}

	var pkExpr string
	if len(pkCols) == 1 {
		pkExpr = QuoteIdent(pkCols[0])
	} else {
		parts := make([]string, len(pkCols))
		for i, c := range pkCols {
			parts[i] = QuoteIdent(c)
		}
		pkExpr = strings.Join(parts, " || '_' || ")
	}

	var pkiExpr string
	if pkIsInt {
		if len(pkCols) == 1 {
			pkiExpr = QuoteIdent(pkCols[0])
		} else {
			pkiExpr = fmt.Sprintf("ABS(hashtext((%s)::text))", pkExpr)
		}
	} else {
		pkiExpr = fmt.Sprintf("ABS(hashtext((%s)::text))", pkExpr)
	}

	s := strings.ReplaceAll(template, "{pk}", pkExpr)
	s = strings.ReplaceAll(s, "{pki}", pkiExpr)
	return s
}

func pkColumnsAreInteger(table *TableInfo) bool {
	for _, pkCol := range table.PrimaryKey {
		col, ok := table.Columns[pkCol]
		if !ok || !isIntegerType(col.Type) {
			return false
		}
	}
	return len(table.PrimaryKey) > 0
}

func FormatAnalysis(schema *DatabaseSchema, result *AnalyzeResult) string {
	if len(result.Matches) == 0 {
		return fmt.Sprintf("No PII columns detected across %d table(s).\n", result.TablesScanned)
	}

	var matchedTables []string
	for t := range result.Matches {
		matchedTables = append(matchedTables, t)
	}
	sorted := schema.TopologicalSort(matchedTables, nil)

	var b strings.Builder
	totalCols := 0

	for i, tableName := range sorted {
		matches := result.Matches[tableName]
		if len(matches) == 0 {
			continue
		}
		if i > 0 {
			b.WriteByte('\n')
		}

		fmt.Fprintf(&b, "%s\n", tableName)

		maxName, maxType, maxCat := 0, 0, 0
		for _, m := range matches {
			if len(m.Column) > maxName {
				maxName = len(m.Column)
			}
			if len(m.ColumnType) > maxType {
				maxType = len(m.ColumnType)
			}
			if len(m.Category) > maxCat {
				maxCat = len(m.Category)
			}
		}

		for _, m := range matches {
			fmt.Fprintf(&b, "  %-*s  %-*s  %-*s  %-4s  %s\n",
				maxName, m.Column,
				maxType, m.ColumnType,
				maxCat, m.Category,
				m.Confidence.String(),
				m.MaskExpr,
			)
			totalCols++
		}
	}

	fmt.Fprintf(&b, "\n%d PII column(s) in %d table(s)\n", totalCols, len(sorted))
	return b.String()
}

func (r *AnalyzeResult) TotalMatches() int {
	total := 0
	for _, matches := range r.Matches {
		total += len(matches)
	}
	return total
}
