package fixturize

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMatchesColumnName(t *testing.T) {
	tests := []struct {
		colName  string
		patterns []string
		want     bool
	}{
		{"email", []string{"email"}, true},
		{"user_email", []string{"email"}, true},
		{"email_address", []string{"email"}, true},
		{"primary_email_address", []string{"email"}, true},
		{"emailed_at", []string{"email"}, false},
		{"emailaddress", []string{"emailaddress"}, true},
		{"email_address", []string{"emailaddress"}, true},
		{"ip_address", []string{"ip", "ipaddress"}, true},
		{"source_ip", []string{"ip"}, true},
		{"zip_code", []string{"zip"}, true},
		{"description", []string{"name"}, false},
		{"first_name", []string{"first"}, true},
		{"firstname", []string{"first"}, false},
		// Compound patterns avoid false positives
		{"first_name", []string{"firstname"}, true},
		{"first_login", []string{"firstname"}, false},
		{"last_name", []string{"lastname"}, true},
		{"last_updated", []string{"lastname"}, false},
		{"display_name", []string{"displayname"}, true},
		{"table_name", []string{"displayname", "fullname"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.colName, func(t *testing.T) {
			got := matchesColumnName(tt.colName, tt.patterns)
			if got != tt.want {
				t.Errorf("matchesColumnName(%q, %v) = %v, want %v", tt.colName, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestMatchesType(t *testing.T) {
	tests := []struct {
		colType      string
		typePatterns []string
		want         bool
	}{
		{"character varying(255)", textTypes, true},
		{"text", textTypes, true},
		{"bigint", textTypes, false},
		{"inet", []string{"inet", "cidr"}, true},
		{"integer", numericTypes, true},
		{"double precision", numericTypes, true},
		{"boolean", textTypes, false},
	}

	for _, tt := range tests {
		t.Run(tt.colType, func(t *testing.T) {
			got := matchesType(tt.colType, tt.typePatterns)
			if got != tt.want {
				t.Errorf("matchesType(%q, ...) = %v, want %v", tt.colType, got, tt.want)
			}
		})
	}
}

func TestResolveMask(t *testing.T) {
	t.Run("single integer PK", func(t *testing.T) {
		result := resolveMask("'user_' || {pk} || '@test.com'", []string{"id"}, true)
		if result != "'user_' || \"id\" || '@test.com'" {
			t.Errorf("got %q", result)
		}
	})

	t.Run("single integer PK arithmetic", func(t *testing.T) {
		result := resolveMask("'+1555' || LPAD(({pki} % 10000000)::text, 7, '0')", []string{"id"}, true)
		if !strings.Contains(result, "\"id\" % 10000000") {
			t.Errorf("expected id in arithmetic, got %q", result)
		}
	})

	t.Run("composite PK", func(t *testing.T) {
		result := resolveMask("'user_' || {pk} || '@test.com'", []string{"tenant_id", "user_id"}, true)
		if !strings.Contains(result, `"tenant_id" || '_' || "user_id"`) {
			t.Errorf("expected concatenated PKs, got %q", result)
		}
	})

	t.Run("composite PK arithmetic uses hashtext", func(t *testing.T) {
		result := resolveMask("LPAD(({pki} % 10000)::text, 4, '0')", []string{"tenant_id", "user_id"}, true)
		if !strings.Contains(result, "hashtext") {
			t.Errorf("expected hashtext for composite PK arithmetic, got %q", result)
		}
	})

	t.Run("UUID PK arithmetic uses hashtext", func(t *testing.T) {
		result := resolveMask("({pki} % 256)", []string{"id"}, false)
		if !strings.Contains(result, "hashtext") {
			t.Errorf("expected hashtext for non-integer PK, got %q", result)
		}
	})

	t.Run("no PK", func(t *testing.T) {
		result := resolveMask("'user_' || {pk} || '@test.com'", nil, false)
		if !strings.Contains(result, "'0'") {
			t.Errorf("expected fallback '0', got %q", result)
		}
	})
}

func TestAnalyzeSchema_SkipsPKAndFK(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":     {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email":  {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
					"org_id": {Name: "org_id", Type: "bigint", OrdinalPosition: 3, IsForeignKey: true},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users"}, ConfidenceLow)
	matches := result.Matches["public.users"]

	for _, m := range matches {
		if m.Column == "id" {
			t.Error("PK column should be skipped")
		}
		if m.Column == "org_id" {
			t.Error("FK column should be skipped")
		}
	}

	found := false
	for _, m := range matches {
		if m.Column == "email" {
			found = true
			if m.Category != "Email" {
				t.Errorf("expected Email category, got %s", m.Category)
			}
			if m.Confidence < ConfidenceHigh {
				t.Errorf("expected HIGH confidence for email varchar (name+type match), got %d", m.Confidence)
			}
		}
	}
	if !found {
		t.Error("expected email column to be detected as PII")
	}
}

func TestAnalyzeSchema_SkipsIntegerNameMatch(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.logs": {
				Schema:     "public",
				Name:       "logs",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":       {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"ip_count": {Name: "ip_count", Type: "integer", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.logs"}, ConfidenceLow)
	for _, m := range result.Matches["public.logs"] {
		if m.Column == "ip_count" {
			t.Error("integer column matched by name only should be skipped")
		}
	}
}

func TestAnalyzeSchema_MinConfidenceFilter(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.people": {
				Schema:     "public",
				Name:       "people",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
					"name":  {Name: "name", Type: "text", OrdinalPosition: 3},
				},
			},
		},
	}

	resultAll := AnalyzeSchema(schema, []string{"public.people"}, ConfidenceLow)
	resultMed := AnalyzeSchema(schema, []string{"public.people"}, ConfidenceMedium)

	if resultAll.TotalMatches() < resultMed.TotalMatches() {
		t.Error("LOW threshold should return >= matches than MEDIUM")
	}
}

func TestFormatAnalysis(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users"}, ConfidenceLow)
	output := FormatAnalysis(schema, result)

	if !strings.Contains(output, "public.users") {
		t.Error("expected table name in output")
	}
	if !strings.Contains(output, "email") {
		t.Error("expected email column in output")
	}
	if !strings.Contains(output, "Email") {
		t.Error("expected Email category in output")
	}
	if !strings.Contains(output, "HIGH") {
		t.Error("expected HIGH confidence for email (name+type match)")
	}
	if !strings.Contains(output, "PII column(s)") {
		t.Error("expected summary line in output")
	}
}

func TestFormatAnalysis_NoMatches(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.settings": {
				Schema:     "public",
				Name:       "settings",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"value": {Name: "value", Type: "jsonb", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.settings"}, ConfidenceLow)
	output := FormatAnalysis(schema, result)

	if !strings.Contains(output, "No PII columns detected") {
		t.Errorf("expected 'No PII columns detected', got: %s", output)
	}
}

func TestConfidenceLevelString(t *testing.T) {
	tests := []struct {
		level ConfidenceLevel
		want  string
	}{
		{ConfidenceHigh, "HIGH"},
		{ConfidenceMedium, "MED"},
		{ConfidenceLow, "LOW"},
		{95, "HIGH"},
		{60, "MED"},
		{10, "LOW"},
	}
	for _, tt := range tests {
		got := tt.level.String()
		if got != tt.want {
			t.Errorf("ConfidenceLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestFormatAnalysisYAML(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	if !strings.Contains(output, "masks:") {
		t.Error("expected masks: key")
	}
	if !strings.Contains(output, "pii:") {
		t.Error("expected pii: key")
	}
	if !strings.Contains(output, "users.email=") {
		t.Error("expected users.email= entry")
	}
	// public. prefix should be stripped
	if strings.Contains(output, "public.users") {
		t.Error("public. prefix should be stripped")
	}
}

func TestFormatAnalysisYAML_NoMatches(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.settings": {
				Schema:     "public",
				Name:       "settings",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"value": {Name: "value", Type: "jsonb", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.settings"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	if !strings.Contains(output, "pii: []") {
		t.Errorf("expected empty pii list, got: %s", output)
	}
}

func TestFormatAnalysisYAML_NonPublicSchema(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"auth.users": {
				Schema:     "auth",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"auth.users"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	if !strings.Contains(output, "auth.users.email=") {
		t.Errorf("expected auth.users.email= with schema prefix, got: %s", output)
	}
}

func TestFormatAnalysisYAML_MultipleColumns(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
					"phone": {Name: "phone", Type: "text", OrdinalPosition: 3},
					"name":  {Name: "name", Type: "text", OrdinalPosition: 4},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	if !strings.Contains(output, "users.email=") {
		t.Error("expected users.email= entry")
	}
	if !strings.Contains(output, "users.phone=") {
		t.Error("expected users.phone= entry")
	}

	// Each entry should be a separate YAML list item
	lines := strings.Split(output, "\n")
	listItems := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			listItems++
		}
	}
	if listItems < 2 {
		t.Errorf("expected at least 2 YAML list entries, got %d", listItems)
	}
}

func TestFormatAnalysisYAML_MultipleTables(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
				},
			},
			"public.contacts": {
				Schema:     "public",
				Name:       "contacts",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"phone": {Name: "phone", Type: "text", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users", "public.contacts"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	if !strings.Contains(output, "users.email=") {
		t.Error("expected users.email= entry")
	}
	if !strings.Contains(output, "contacts.phone=") {
		t.Error("expected contacts.phone= entry")
	}
}

func TestFormatAnalysisYAML_EscapesDoubleQuotes(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	// Mask expressions contain "id" (quoted identifiers), those must be escaped
	// The entry is wrapped in double quotes, so inner " becomes \"
	if !strings.Contains(output, `\"id\"`) {
		t.Errorf("expected escaped double quotes in mask expression, got: %s", output)
	}
}

func TestFormatAnalysisYAML_ValidYAML(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
					"phone": {Name: "phone", Type: "text", OrdinalPosition: 3},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	var parsed struct {
		Masks map[string][]string `yaml:"masks"`
	}
	if err := yaml.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("YAML output is not valid: %v\nOutput:\n%s", err, output)
	}

	pii := parsed.Masks["pii"]
	if len(pii) == 0 {
		t.Fatal("expected non-empty pii list after parsing")
	}

	foundEmail := false
	for _, entry := range pii {
		if strings.HasPrefix(entry, "users.email=") {
			foundEmail = true
		}
	}
	if !foundEmail {
		t.Errorf("expected users.email= in parsed YAML entries, got: %v", pii)
	}
}

func TestFormatAnalysisYAML_ValidYAML_NoMatches(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.settings": {
				Schema:     "public",
				Name:       "settings",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"value": {Name: "value", Type: "jsonb", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.settings"}, ConfidenceLow)
	output := FormatAnalysisYAML(schema, result)

	var parsed struct {
		Masks map[string][]string `yaml:"masks"`
	}
	if err := yaml.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("empty YAML output is not valid: %v\nOutput:\n%s", err, output)
	}

	if len(parsed.Masks["pii"]) != 0 {
		t.Errorf("expected empty pii list, got: %v", parsed.Masks["pii"])
	}
}

func TestShortTableName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"public.users", "users"},
		{"public.orders", "orders"},
		{"auth.users", "auth.users"},
		{"billing.invoices", "billing.invoices"},
		{"users", "users"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := shortTableName(tt.input)
			if got != tt.want {
				t.Errorf("shortTableName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAnalyzeSchema_UnknownTable(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":    {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email": {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users", "public.nonexistent"}, ConfidenceLow)

	if result.TablesScanned != 2 {
		t.Errorf("expected 2 tables scanned, got %d", result.TablesScanned)
	}
	if _, ok := result.Matches["public.nonexistent"]; ok {
		t.Error("unknown table should have no matches")
	}
	if len(result.Matches["public.users"]) == 0 {
		t.Error("known table should still have matches")
	}
}

func TestTotalMatches(t *testing.T) {
	result := &AnalyzeResult{
		Matches: map[string][]PIIMatch{
			"public.users":    {{Column: "email"}, {Column: "phone"}},
			"public.contacts": {{Column: "address"}},
		},
	}

	if got := result.TotalMatches(); got != 3 {
		t.Errorf("TotalMatches() = %d, want 3", got)
	}
}

func TestTotalMatches_Empty(t *testing.T) {
	result := &AnalyzeResult{
		Matches: map[string][]PIIMatch{},
	}

	if got := result.TotalMatches(); got != 0 {
		t.Errorf("TotalMatches() = %d, want 0", got)
	}
}

func TestAnalyzeSchema_SkipsGenerated(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.users": {
				Schema:     "public",
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":           {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"email":        {Name: "email", Type: "character varying(255)", OrdinalPosition: 2},
					"display_name": {Name: "display_name", Type: "text", OrdinalPosition: 3, IsGenerated: true},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.users"}, ConfidenceLow)
	matches := result.Matches["public.users"]

	for _, m := range matches {
		if m.Column == "display_name" {
			t.Error("generated column should be skipped")
		}
	}

	found := false
	for _, m := range matches {
		if m.Column == "email" {
			found = true
		}
	}
	if !found {
		t.Error("non-generated PII column should still be detected")
	}
}

func TestAnalyzeSchema_APIKeyRules(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.configs": {
				Schema:     "public",
				Name:       "configs",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":             {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"api_key":        {Name: "api_key", Type: "character varying(255)", OrdinalPosition: 2},
					"secret_key":     {Name: "secret_key", Type: "text", OrdinalPosition: 3},
					"access_token":   {Name: "access_token", Type: "character varying(255)", OrdinalPosition: 4},
					"refresh_token":  {Name: "refresh_token", Type: "text", OrdinalPosition: 5},
					"token":          {Name: "token", Type: "text", OrdinalPosition: 6},
					"client_secret":  {Name: "client_secret", Type: "varchar(500)", OrdinalPosition: 7},
					"webhook_secret": {Name: "webhook_secret", Type: "text", OrdinalPosition: 8},
					"api_version":    {Name: "api_version", Type: "text", OrdinalPosition: 9},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.configs"}, ConfidenceLow)
	matches := result.Matches["public.configs"]

	expected := map[string]struct {
		category   string
		confidence ConfidenceLevel
	}{
		"api_key":        {"API Key", ConfidenceHigh},
		"secret_key":     {"API Key", ConfidenceHigh},
		"access_token":   {"Access Token", ConfidenceHigh},
		"refresh_token":  {"Access Token", ConfidenceHigh},
		"token":          {"Access Token", ConfidenceHigh},
		"client_secret":  {"API Key", ConfidenceHigh},
		"webhook_secret": {"API Key", ConfidenceHigh},
	}

	found := make(map[string]bool)
	for _, m := range matches {
		found[m.Column] = true
		exp, ok := expected[m.Column]
		if !ok {
			continue
		}
		if m.Category != exp.category {
			t.Errorf("%s: category = %q, want %q", m.Column, m.Category, exp.category)
		}
		if m.Confidence < exp.confidence {
			t.Errorf("%s: confidence = %d, want >= %d", m.Column, m.Confidence, exp.confidence)
		}
	}

	for col := range expected {
		if !found[col] {
			t.Errorf("expected column %q to be detected as PII", col)
		}
	}

	// api_version should NOT match any PII rule
	if found["api_version"] {
		t.Error("api_version should not be detected as PII")
	}
}

func TestAnalyzeSchema_APIKeyMaskExpressions(t *testing.T) {
	schema := &DatabaseSchema{
		tables: map[string]*TableInfo{
			"public.settings": {
				Schema:     "public",
				Name:       "settings",
				PrimaryKey: []string{"id"},
				Columns: map[string]*ColumnInfo{
					"id":           {Name: "id", Type: "bigint", OrdinalPosition: 1, IsPrimaryKey: true},
					"api_key":      {Name: "api_key", Type: "text", OrdinalPosition: 2},
					"access_token": {Name: "access_token", Type: "text", OrdinalPosition: 3},
				},
			},
		},
	}

	result := AnalyzeSchema(schema, []string{"public.settings"}, ConfidenceLow)
	for _, m := range result.Matches["public.settings"] {
		switch m.Column {
		case "api_key":
			if !strings.Contains(m.MaskExpr, "sk_test_") {
				t.Errorf("api_key mask should contain 'sk_test_', got %q", m.MaskExpr)
			}
		case "access_token":
			if !strings.Contains(m.MaskExpr, "tok_test_") {
				t.Errorf("access_token mask should contain 'tok_test_', got %q", m.MaskExpr)
			}
		}
	}
}

func TestParseConfidence_Invalid(t *testing.T) {
	_, err := ParseConfidence("invalid")
	if err == nil {
		t.Error("expected error for invalid confidence level")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected descriptive error, got: %s", err)
	}
}
