package cmd

import (
	"fmt"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	inspectCmd = &cobra.Command{
		Use:   "inspect",
		Short: "Display schema structure with FK relationships",
		Long: `Introspect a PostgreSQL database schema and display tables, columns,
primary keys, foreign keys, and unique constraints.

Useful for understanding FK relationships and planning extractions
without extracting any data.

Examples:
  # Show all tables
  fixturize inspect --connection "$DB"

  # Show only tables reachable from users (2 FK hops)
  fixturize inspect --connection "$DB" --root users --depth 2

  # Inspect a non-public schema
  fixturize inspect --connection "$DB" --schema myapp`,
		RunE: runInspect,
	}

	inspectConn   string
	inspectSchema string
	inspectRoot   string
	inspectDepth  int
)

func init() {
	RootCmd.AddCommand(inspectCmd)

	inspectCmd.Flags().StringVar(&inspectConn, "connection", "", "PostgreSQL connection string")
	inspectCmd.Flags().StringVar(&inspectSchema, "schema", "", "Default schema for unqualified names (default: public)")
	inspectCmd.Flags().StringVar(&inspectRoot, "root", "", "Filter to tables reachable from this root table")
	inspectCmd.Flags().IntVar(&inspectDepth, "depth", 0, "Max FK hops from root (0 = unlimited, only with --root)")
}

func runInspect(cmd *cobra.Command, args []string) error {
	conn, schema, err := resolveConnAndSchema(cmd, inspectConn, inspectSchema)
	if err != nil {
		return err
	}
	root := inspectRoot
	depth := inspectDepth

	db, err := fixturize.OpenDB(conn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	dbSchema, err := fixturize.IntrospectSchema(db)
	if err != nil {
		return fmt.Errorf("failed to introspect schema: %w", err)
	}

	var tables []string
	if root != "" {
		// Qualify root if needed
		if schema != "" && root != "" && !containsDot(root) {
			root = schema + "." + root
		}
		tables, err = fixturize.ReachableSubgraph(dbSchema, root, depth)
		if err != nil {
			return err
		}
	} else {
		tables = dbSchema.GetTables()
	}

	fmt.Print(fixturize.FormatInspect(dbSchema, tables))
	return nil
}

func containsDot(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return true
		}
	}
	return false
}
