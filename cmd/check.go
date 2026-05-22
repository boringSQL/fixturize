package cmd

import (
	"fmt"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	checkCmd = &cobra.Command{
		Use:   "check",
		Short: "Find rows that violate foreign key relationships",
		Long: `Scan every foreign key constraint and report child rows that point at a
parent row which no longer exists.

PostgreSQL normally prevents orphaned rows, but they show up via NOT VALID
constraints, FKs added after data was loaded, or constraints that were
dropped and re-created loosely.

Examples:
  # Check the whole database
  fixturize check --connection "$DB"

  # Only tables reachable from users (2 FK hops)
  fixturize check --connection "$DB" --root users --depth 2

  # Print DELETE statements to clean up the orphans
  fixturize check --connection "$DB" --emit-sql

The command exits non-zero when orphaned rows are found, so it can gate CI.`,
		RunE: runCheck,
	}

	checkConn    string
	checkSchema  string
	checkRoot    string
	checkDepth   int
	checkSamples int
	checkEmitSQL bool
	checkTimeout int
	checkVerbose bool
)

func init() {
	RootCmd.AddCommand(checkCmd)

	checkCmd.Flags().StringVar(&checkConn, "connection", "", "PostgreSQL connection string")
	checkCmd.Flags().StringVar(&checkSchema, "schema", "", "Default schema for unqualified names (default: public)")
	checkCmd.Flags().StringVar(&checkRoot, "root", "", "Filter to tables reachable from this root table")
	checkCmd.Flags().IntVar(&checkDepth, "depth", 0, "Max FK hops from root (0 = unlimited, only with --root)")
	checkCmd.Flags().IntVar(&checkSamples, "samples", 3, "Offending rows to show per constraint (0 = counts only)")
	checkCmd.Flags().BoolVar(&checkEmitSQL, "emit-sql", false, "Print DELETE statements for orphaned rows instead of a report")
	checkCmd.Flags().IntVar(&checkTimeout, "statement-timeout", 30, "Per-query timeout in seconds")
	checkCmd.Flags().BoolVar(&checkVerbose, "verbose", false, "Print per-constraint progress with timings as the scan runs")
}

func runCheck(cmd *cobra.Command, args []string) error {
	conn, schema, err := resolveConnAndSchema(cmd, checkConn, checkSchema)
	if err != nil {
		return err
	}
	root := checkRoot
	depth := checkDepth

	db, err := fixturize.OpenDB(cmd.Context(), conn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	dbSchema, err := fixturize.IntrospectSchema(cmd.Context(), db)
	if err != nil {
		return fmt.Errorf("failed to introspect schema: %w", err)
	}

	var tables []string
	if root != "" {
		if schema != "" && !containsDot(root) {
			root = schema + "." + root
		}
		tables, err = fixturize.ReachableSubgraph(dbSchema, root, depth)
		if err != nil {
			return err
		}
	} else {
		tables = dbSchema.GetTables()
	}

	results, err := fixturize.CheckOrphans(cmd.Context(), db, dbSchema, tables, fixturize.CheckOptions{
		Samples:          checkSamples,
		StatementTimeout: checkTimeout,
		Verbose:          checkVerbose,
	})
	if err != nil {
		return err
	}

	if checkVerbose && !checkEmitSQL {
		fmt.Println()
	}

	if checkEmitSQL {
		fmt.Print(fixturize.FormatCheckSQL(results))
	} else {
		fmt.Print(fixturize.FormatCheck(results))
	}

	// non-zero exit so the command can fail a CI pipeline
	if total := fixturize.TotalOrphans(results); total > 0 {
		cmd.SilenceUsage = true
		return fmt.Errorf("integrity check failed: %d orphaned row(s)", total)
	}

	return nil
}
