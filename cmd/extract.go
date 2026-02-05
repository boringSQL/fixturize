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

	extractCmd.Flags().StringVar(&extractConn, "connection", "", "PostgreSQL connection string")
	extractCmd.Flags().StringVar(&extractRoot, "root", "", "Root table + optional WHERE/ORDER BY/LIMIT")
	extractCmd.Flags().StringVar(&extractSchema, "schema", "", "Default schema for unqualified names")
	extractCmd.Flags().StringVarP(&extractOutput, "output", "o", "", "Output file path (default: extracted.json)")
	extractCmd.Flags().IntVar(&extractLimit, "limit", 0, "Max rows per child table (0 = unlimited)")
	extractCmd.Flags().IntVar(&extractDepth, "depth", 0, "Max FK hops from root (0 = follow everything)")
	extractCmd.Flags().StringVar(&extractInclude, "include", "", "Extra tables to include (comma-separated)")
	extractCmd.Flags().StringVar(&extractExclude, "exclude", "", "Tables to skip (comma-separated)")
	extractCmd.Flags().StringArrayVar(&extractMask, "mask", nil, "Mask column with SQL expression (table.column=expr, repeatable)")
	extractCmd.Flags().IntVar(&extractStatementTimeout, "statement-timeout", 0, "Per-statement timeout in seconds")
	extractCmd.Flags().BoolVar(&extractDryRun, "dry-run", false, "Print JSON to stdout, don't write file")
}

func expandEnvVars(s string) string {
	return os.Expand(s, os.Getenv)
}

func runExtract(cmd *cobra.Command, args []string) error {
	conn, root, schema, output, limit, depth, include, exclude, masks, statementTimeout := mergeExtractConfig(cmd)

	if conn == "" {
		conn = os.Getenv("DATABASE_URL")
	}
	if conn == "" {
		return fmt.Errorf("connection string is required (use --connection, DATABASE_URL env var, or profile)")
	}
	if root == "" {
		return fmt.Errorf("root is required (use --root or profile)")
	}

	conn = expandEnvVars(conn)
	db, err := fixturize.OpenDB(conn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	options := &fixturize.ExtractOptions{
		Connection:       conn,
		Root:             root,
		Schema:           schema,
		Output:           output,
		Limit:            limit,
		Depth:            depth,
		Include:          include,
		Exclude:          exclude,
		Mask:             masks,
		StatementTimeout: statementTimeout,
		DryRun:           extractDryRun,
	}

	// Delete previous output before extraction so stale data can't persist on failure
	if !extractDryRun && output != "" {
		os.Remove(output)
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

	outputPath := output
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

func mergeExtractConfig(cmd *cobra.Command) (conn, root, schema, output string, limit, depth int, include, exclude, masks []string, statementTimeout int) {
	conn = extractConn
	root = extractRoot
	schema = extractSchema
	output = extractOutput
	limit = extractLimit
	depth = extractDepth
	include = parseCommaSeparated(extractInclude)
	exclude = parseCommaSeparated(extractExclude)
	masks = extractMask
	statementTimeout = extractStatementTimeout

	if loadedProfile == nil {
		if schema == "" {
			schema = "public"
		}
		if statementTimeout == 0 {
			statementTimeout = 30
		}
		return
	}

	p := loadedProfile

	if !cmd.Flags().Changed("connection") && conn == "" {
		conn = p.Connection
	}
	if !cmd.Flags().Changed("root") && root == "" {
		root = p.Extract.Root
	}
	if !cmd.Flags().Changed("schema") {
		if schema == "" {
			schema = p.Schema
		}
	}
	if schema == "" {
		schema = "public"
	}
	if !cmd.Flags().Changed("output") && output == "" {
		output = p.Extract.Output
	}
	if !cmd.Flags().Changed("limit") && limit == 0 {
		limit = p.Extract.Limit
	}
	if !cmd.Flags().Changed("depth") && depth == 0 {
		depth = p.Extract.Depth
	}
	if !cmd.Flags().Changed("statement-timeout") {
		if statementTimeout == 0 {
			statementTimeout = p.Extract.StatementTimeout
		}
	}
	if statementTimeout == 0 {
		statementTimeout = 30
	}
	if !cmd.Flags().Changed("include") && len(include) == 0 {
		include = p.Extract.Include
	}
	if !cmd.Flags().Changed("exclude") && len(exclude) == 0 {
		exclude = p.Extract.Exclude
	}
	if !cmd.Flags().Changed("mask") && len(masks) == 0 {
		masks = p.ResolveMasks()
	}

	return
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
