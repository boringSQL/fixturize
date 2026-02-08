package fixturize

import "testing"

func TestQuoteIdent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "id", `"id"`},
		{"with space", "my column", `"my column"`},
		{"with double quote", `say"hello`, `"say""hello"`},
		{"reserved word", "select", `"select"`},
		{"empty", "", `""`},
		{"multiple quotes", `a"b"c`, `"a""b""c"`},
		{"underscore", "first_name", `"first_name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteIdent(tt.in)
			if got != tt.want {
				t.Errorf("QuoteIdent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestQuoteQualifiedTable(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"public schema", "public.users", `"public"."users"`},
		{"custom schema", "auth.users", `"auth"."users"`},
		{"no schema defaults to public", "users", `"public"."users"`},
		{"quotes in names", `my"schema.my"table`, `"my""schema"."my""table"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteQualifiedTable(tt.in)
			if got != tt.want {
				t.Errorf("QuoteQualifiedTable(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestQuoteColumns(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"single", []string{"id"}, `"id"`},
		{"multiple", []string{"id", "name", "email"}, `"id", "name", "email"`},
		{"with special chars", []string{`a"b`, "c d"}, `"a""b", "c d"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteColumns(tt.in)
			if got != tt.want {
				t.Errorf("QuoteColumns(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
