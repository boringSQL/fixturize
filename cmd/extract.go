package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	extractCmd = &cobra.Command{
		Use:   "extract",
		Short: "Extract a consistent subgraph of real data from a live database",
		Long: `Extract real data from a PostgreSQL database by following foreign key
relationships from a root table query. Produces a self-contained JSON fixture
with data that satisfies all FK constraints by definition.

Examples:
  # Extract one organization and everything related
  fixturize extract --connection "$DB" \
    --root "organizations WHERE id = 42"

  # Extract 3 random orgs, cap children at 500 rows each
  fixturize extract --connection "$DB" \
    --root "organizations ORDER BY random() LIMIT 3" \
    --limit 500

  # Include lookup tables, exclude audit logs
  fixturize extract --connection "$DB" \
    --root "organizations WHERE name = 'acme'" \
    --include "roles,permissions" \
    --exclude "audit_log,event_log"

  # Mask PII columns
  fixturize extract --connection "$DB" \
    --root "organizations WHERE id = 42" \
    --mask "auth.users.email='user_' || id || '@test.com'" \
    --mask "auth.users.name='User ' || id"

  # Preview without writing
  fixturize extract --connection "$DB" \
    --root "users LIMIT 5" --dry-run`,
		RunE: runExtract,
	}

	extractConn             string
	extractRoot             string
	extractSchema           string
	extractOutput           string
	extractLimit            int
	extractDepth            int
	extractInclude          string
	extractExclude          string
	extractMask             []string
	extractStatementTimeout int
	extractDryRun           bool
)

func init() {
	RootCmd.AddCommand(extractCmd)

	extractCmd.Flags().StringVar(&extractConn, "connection", "", "PostgreSQL connection string (required)")
	extractCmd.Flags().StringVar(&extractRoot, "root", "", "Root table + optional WHERE/ORDER BY/LIMIT (required)")
	extractCmd.Flags().StringVar(&extractSchema, "schema", "public", "Default schema for unqualified names")
	extractCmd.Flags().StringVarP(&extractOutput, "output", "o", "", "Output file path (default: extracted.json)")
	extractCmd.Flags().IntVar(&extractLimit, "limit", 0, "Max rows per child table (0 = unlimited)")
	extractCmd.Flags().IntVar(&extractDepth, "depth", 0, "Max FK hops from root (0 = follow everything)")
	extractCmd.Flags().StringVar(&extractInclude, "include", "", "Extra tables to include (comma-separated)")
	extractCmd.Flags().StringVar(&extractExclude, "exclude", "", "Tables to skip (comma-separated)")
	extractCmd.Flags().StringArrayVar(&extractMask, "mask", nil, "Mask column with SQL expression (table.column=expr, repeatable)")
	extractCmd.Flags().IntVar(&extractStatementTimeout, "statement-timeout", 30, "Per-statement timeout in seconds")
	extractCmd.Flags().BoolVar(&extractDryRun, "dry-run", false, "Print JSON to stdout, don't write file")
	extractCmd.MarkFlagRequired("connection")
	extractCmd.MarkFlagRequired("root")
}

func runExtract(cmd *cobra.Command, args []string) error {
	db, err := fixturize.OpenDB(extractConn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	options := &fixturize.ExtractOptions{
		Connection:       extractConn,
		Root:             extractRoot,
		Schema:           extractSchema,
		Output:           extractOutput,
		Limit:            extractLimit,
		Depth:            extractDepth,
		Include:          parseCommaSeparated(extractInclude),
		Exclude:          parseCommaSeparated(extractExclude),
		Mask:             extractMask,
		StatementTimeout: extractStatementTimeout,
		DryRun:           extractDryRun,
	}

	// Delete previous output before extraction so stale data can't persist on failure
	if !extractDryRun && extractOutput != "" {
		os.Remove(extractOutput)
	}

	extractor := fixturize.NewExtractor(db, options)
	result, err := extractor.Extract()
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	if extractDryRun {
		fmt.Println(string(result.JSON))
		return nil
	}

	outputPath := extractOutput
	if outputPath == "" {
		outputPath = "extracted.json"
	}

	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	if err := os.WriteFile(outputPath, result.JSON, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Fixture written to: %s\n", outputPath)
	return nil
}

func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			p := trimSpace(s[start:i])
			if p != "" {
				parts = append(parts, p)
			}
			start = i + 1
		}
	}
	p := trimSpace(s[start:])
	if p != "" {
		parts = append(parts, p)
	}
	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
