package cmd

import (
	"fmt"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	suggestCmd = &cobra.Command{
		Use:   "suggest-roots",
		Short: "Suggest best root table candidates for extract",
		Long: `Analyze the FK graph and score tables by how suitable they are
as root tables for extract. Tables are ranked by child count, reachability,
and outgoing FK penalty.

Examples:
  fixturize suggest-roots --connection "$DB"
  fixturize suggest-roots --connection "$DB" --top 5`,
		RunE: runSuggestRoots,
	}

	suggestConn   string
	suggestSchema string
	suggestTop    int
)

func init() {
	RootCmd.AddCommand(suggestCmd)

	suggestCmd.Flags().StringVar(&suggestConn, "connection", "", "PostgreSQL connection string")
	suggestCmd.Flags().StringVar(&suggestSchema, "schema", "", "Default schema for unqualified names (default: public)")
	suggestCmd.Flags().IntVar(&suggestTop, "top", 10, "Number of top candidates to show (0 = all)")
}

func runSuggestRoots(cmd *cobra.Command, args []string) error {
	conn, _, err := resolveConnAndSchema(cmd, suggestConn, suggestSchema)
	if err != nil {
		return err
	}

	db, err := fixturize.OpenDB(conn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	dbSchema, err := fixturize.IntrospectSchema(db)
	if err != nil {
		return fmt.Errorf("failed to introspect schema: %w", err)
	}

	candidates := dbSchema.SuggestRootTables()

	if suggestTop > 0 && len(candidates) > suggestTop {
		candidates = candidates[:suggestTop]
	}

	fmt.Print(fixturize.FormatSuggestRoots(candidates))
	return nil
}
