package cmd

import (
	"fmt"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	suggestFKsCmd = &cobra.Command{
		Use:   "suggest-fks",
		Short: "Find columns that look like foreign keys but have no FK constraint",
		Long: `Looks for *_id columns with no foreign key constraint and matches each
to a likely parent table by name and type.

Unless --names-only is set, every match is probed against the data. A
match is safe to add when all values have a parent row, otherwise the
orphaned rows are reported.

Examples:
  # scan the whole database
  fixturize suggest-fks --connection "$DB"

  # name and type matching only, no data probe
  fixturize suggest-fks --connection "$DB" --names-only

  # also show *_id columns with no parent match
  fixturize suggest-fks --connection "$DB" --show-unmatched

  # print ALTER TABLE statements (NOT VALID where rows are orphaned)
  fixturize suggest-fks --connection "$DB" --emit-sql`,
		RunE: runSuggestFKs,
	}

	suggestFKsConn      string
	suggestFKsSchema    string
	suggestFKsRoot      string
	suggestFKsDepth     int
	suggestFKsNamesOnly bool
	suggestFKsUnmatched bool
	suggestFKsEmitSQL   bool
	suggestFKsTimeout   int
	suggestFKsVerbose   bool
)

func init() {
	RootCmd.AddCommand(suggestFKsCmd)

	suggestFKsCmd.Flags().StringVar(&suggestFKsConn, "connection", "", "PostgreSQL connection string")
	suggestFKsCmd.Flags().StringVar(&suggestFKsSchema, "schema", "", "Default schema for unqualified names (default: public)")
	suggestFKsCmd.Flags().StringVar(&suggestFKsRoot, "root", "", "Filter to tables reachable from this root table")
	suggestFKsCmd.Flags().IntVar(&suggestFKsDepth, "depth", 0, "Max FK hops from root (0 = unlimited, only with --root)")
	suggestFKsCmd.Flags().BoolVar(&suggestFKsNamesOnly, "names-only", false, "Skip the data probe, report name and type matches only")
	suggestFKsCmd.Flags().BoolVar(&suggestFKsUnmatched, "show-unmatched", false, "Also list *_id columns with no candidate parent")
	suggestFKsCmd.Flags().BoolVar(&suggestFKsEmitSQL, "emit-sql", false, "Print ALTER TABLE statements instead of a report")
	suggestFKsCmd.Flags().IntVar(&suggestFKsTimeout, "statement-timeout", 30, "Per-query timeout in seconds")
	suggestFKsCmd.Flags().BoolVar(&suggestFKsVerbose, "verbose", false, "Print per-candidate probe progress with timings")
}

func runSuggestFKs(cmd *cobra.Command, args []string) error {
	conn, schema, err := resolveConnAndSchema(cmd, suggestFKsConn, suggestFKsSchema)
	if err != nil {
		return err
	}
	root := suggestFKsRoot
	depth := suggestFKsDepth

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

	suggestions := fixturize.SuggestMissingFKs(dbSchema, tables)

	if !suggestFKsNamesOnly {
		if err := fixturize.ValidateFKSuggestions(cmd.Context(), db, suggestions, fixturize.SuggestFKOptions{
			Validate:         true,
			StatementTimeout: suggestFKsTimeout,
			Verbose:          suggestFKsVerbose,
		}); err != nil {
			return err
		}
		if suggestFKsVerbose && !suggestFKsEmitSQL {
			fmt.Println()
		}
	}

	if suggestFKsEmitSQL {
		fmt.Print(fixturize.FormatSuggestFKsSQL(suggestions))
	} else {
		fmt.Print(fixturize.FormatSuggestFKs(suggestions, suggestFKsUnmatched))
	}

	return nil
}
