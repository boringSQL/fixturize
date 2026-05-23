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

  # Output as SQL INSERT statements
  fixturize extract --connection "$DB" \
    --root "organizations WHERE id = 42" \
    --format sql --transaction --on-conflict-do-nothing

  # Preview without writing
  fixturize extract --connection "$DB" \
    --root "users LIMIT 5" --dry-run`,
		RunE: runExtract,
	}

	extractConn                string
	extractRoot                string
	extractSeed                string
	extractSchema              string
	extractOutput              string
	extractFormat              string
	extractLimit               int
	extractDepth               int
	extractInclude             string
	extractExclude             string
	extractMask                []string
	extractMaskPolicy          []string
	extractMasksFile           string
	extractNoMasks             bool
	extractFilter              []string
	extractStatementTimeout    int
	extractTransaction         bool
	extractOnConflictDoNothing bool
	extractDryRun              bool
	extractVerbose             bool
)

func init() {
	RootCmd.AddCommand(extractCmd)

	extractCmd.Flags().StringVar(&extractConn, "connection", "", "PostgreSQL connection string")
	extractCmd.Flags().StringVar(&extractRoot, "root", "", "Root table + optional WHERE/ORDER BY/LIMIT")
	extractCmd.Flags().StringVar(&extractSeed, "seed", "", "Seed column=value to discover and extract all matching tables (mutually exclusive with --root)")
	extractCmd.Flags().StringVar(&extractSchema, "schema", "", "Default schema for unqualified names")
	extractCmd.Flags().StringVarP(&extractOutput, "output", "o", "", "Output file path (default: extracted.json or extracted.sql)")
	extractCmd.Flags().StringVar(&extractFormat, "format", "json", "Output format: json or sql")
	extractCmd.Flags().IntVar(&extractLimit, "limit", 0, "Max rows per child table (0 = unlimited)")
	extractCmd.Flags().IntVar(&extractDepth, "depth", 0, "Max FK hops from root (0 = follow everything)")
	extractCmd.Flags().StringVar(&extractInclude, "include", "", "Extra tables to include (comma-separated)")
	extractCmd.Flags().StringVar(&extractExclude, "exclude", "", "Tables to skip (comma-separated)")
	extractCmd.Flags().StringArrayVar(&extractMask, "mask", nil, "Mask column with SQL expression (table.column=expr, repeatable)")
	extractCmd.Flags().StringVar(&extractMasksFile, "masks-file", "", "Path to shared masks file (overrides profile.masks_file and walk-up discovery)")
	extractCmd.Flags().StringArrayVar(&extractMaskPolicy, "mask-policy", nil, "Mask policy name to apply (repeatable, overrides profile.extract.mask_policies)")
	extractCmd.Flags().BoolVar(&extractNoMasks, "no-masks", false, "Disable all mask resolution (CLI, profile, discovery)")
	extractCmd.Flags().StringArrayVar(&extractFilter, "filter", nil, "Per-table WHERE condition (table=expr, repeatable)")
	extractCmd.Flags().IntVar(&extractStatementTimeout, "statement-timeout", 0, "Per-statement timeout in seconds")
	extractCmd.Flags().BoolVar(&extractTransaction, "transaction", false, "Wrap SQL output in BEGIN/COMMIT")
	extractCmd.Flags().BoolVar(&extractOnConflictDoNothing, "on-conflict-do-nothing", false, "Append ON CONFLICT DO NOTHING to SQL INSERTs")
	extractCmd.Flags().BoolVar(&extractDryRun, "dry-run", false, "Print output to stdout, don't write file")
	extractCmd.Flags().BoolVarP(&extractVerbose, "verbose", "v", false, "Print FK edges and lookup counts during traversal")
}

func expandEnvVars(s string) string {
	return os.Expand(s, os.Getenv)
}

func runExtract(cmd *cobra.Command, args []string) error {
	conn, root, seed, schema, output, limit, depth, include, exclude, filters, statementTimeout := mergeExtractConfig(cmd)
	format, transaction, onConflictDN := mergeFormatConfig(cmd)

	masks, err := resolveExtractMasks(cmd)
	if err != nil {
		return err
	}

	if conn == "" {
		conn = os.Getenv("DATABASE_URL")
	}
	if conn == "" {
		return fmt.Errorf("connection string is required (use --connection, DATABASE_URL env var, or profile)")
	}
	if root != "" && seed != "" {
		return fmt.Errorf("--root and --seed are mutually exclusive")
	}
	if root == "" && seed == "" {
		return fmt.Errorf("either --root or --seed is required")
	}

	conn = expandEnvVars(conn)
	db, err := fixturize.OpenDB(cmd.Context(), conn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	options := &fixturize.ExtractOptions{
		Connection:       conn,
		Root:             root,
		Seed:             seed,
		Schema:           schema,
		Output:           output,
		Limit:            limit,
		Depth:            depth,
		Include:          include,
		Exclude:          exclude,
		Mask:             masks,
		Filter:           filters,
		StatementTimeout: statementTimeout,
		DryRun:           extractDryRun,
		Verbose:          extractVerbose,
	}

	// Delete previous output before extraction so stale data can't persist on failure
	if !extractDryRun && output != "" {
		os.Remove(output)
	}

	extractor := fixturize.NewExtractor(cmd.Context(), db, options)
	result, err := extractor.Extract()
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	var outputData []byte
	switch format {
	case "sql":
		outputData = result.Fixture.ToSQL(fixturize.SQLOptions{
			Transaction:  transaction,
			OnConflictDN: onConflictDN,
		})
	default:
		outputData = result.JSON
	}

	if extractDryRun {
		fmt.Println(string(outputData))
		return nil
	}

	outputPath := output
	if outputPath == "" {
		if format == "sql" {
			outputPath = "extracted.sql"
		} else {
			outputPath = "extracted.json"
		}
	}

	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Fixture written to: %s\n", outputPath)
	return nil
}

func mergeExtractConfig(cmd *cobra.Command) (conn, root, seed, schema, output string, limit, depth int, include, exclude, filters []string, statementTimeout int) {
	conn = extractConn
	root = extractRoot
	seed = extractSeed
	schema = extractSchema
	output = extractOutput
	limit = extractLimit
	depth = extractDepth
	include = parseCommaSeparated(extractInclude)
	exclude = parseCommaSeparated(extractExclude)
	filters = extractFilter
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
	if !cmd.Flags().Changed("seed") && seed == "" {
		seed = p.Extract.Seed
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
	if !cmd.Flags().Changed("filter") && len(filters) == 0 {
		for table, expr := range p.Extract.Filters {
			filters = append(filters, table+"="+expr)
		}
	}

	return
}

// --mask is additive; --no-masks suppresses file+profile lookup but keeps --mask entries
func resolveExtractMasks(cmd *cobra.Command) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	opts := fixturize.ResolveMasksOptions{
		FlagFile:     extractMasksFile,
		FlagPolicies: extractMaskPolicy,
		Disabled:     extractNoMasks,
		Cwd:          cwd,
	}

	resolved, err := fixturize.ResolveMasks(loadedProfile, opts)
	if err != nil {
		return nil, err
	}

	if len(extractMask) == 0 {
		return resolved, nil
	}
	return append(append([]string{}, resolved...), extractMask...), nil
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

func mergeFormatConfig(cmd *cobra.Command) (format string, transaction, onConflictDN bool) {
	format = extractFormat
	transaction = extractTransaction
	onConflictDN = extractOnConflictDoNothing

	if loadedProfile == nil {
		return
	}

	p := loadedProfile
	if !cmd.Flags().Changed("format") && format == "json" && p.Extract.Format != "" {
		format = p.Extract.Format
	}
	if !cmd.Flags().Changed("transaction") && !transaction {
		transaction = p.Extract.Transaction
	}
	if !cmd.Flags().Changed("on-conflict-do-nothing") && !onConflictDN {
		onConflictDN = p.Extract.OnConflictDoNothing
	}

	return
}
