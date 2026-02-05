package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	applyCmd = &cobra.Command{
		Use:   "apply <fixture.json>",
		Short: "Apply a JSON fixture to a database",
		Long: `Apply a previously extracted JSON fixture to a PostgreSQL database.
Inserts rows in FK-dependency order.

Examples:
  # Apply fixture to test database
  fixturize apply --connection "$TEST_DB" customer_12345.json

  # Truncate existing data first
  fixturize apply --connection "$TEST_DB" --force customer_12345.json

  # Preview without making changes
  fixturize apply --connection "$TEST_DB" --dry-run customer_12345.json`,
		Args: cobra.ExactArgs(1),
		RunE: runApply,
	}

	applyConn            string
	applyForce           bool
	applyDryRun          bool
	applyDisableTriggers bool
)

func init() {
	RootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringVar(&applyConn, "connection", "", "PostgreSQL connection string")
	applyCmd.Flags().BoolVar(&applyForce, "force", false, "Truncate tables before applying fixture")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Show what would be done without making changes")
	applyCmd.Flags().BoolVar(&applyDisableTriggers, "disable-triggers", false, "Disable triggers during insert (uses replica mode)")
}

func runApply(cmd *cobra.Command, args []string) error {
	if applyConn == "" {
		applyConn = os.Getenv("DATABASE_URL")
	}
	if applyConn == "" {
		return fmt.Errorf("connection string is required (use --connection or DATABASE_URL env var)")
	}

	conn := expandEnvVars(applyConn)
	db, err := fixturize.OpenDB(conn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	options := &fixturize.ApplyOptions{
		Connection:      conn,
		Fixture:         args[0],
		Force:           applyForce,
		DryRun:          applyDryRun,
		DisableTriggers: applyDisableTriggers,
	}

	result, err := fixturize.ApplyFixtureFile(db, options)
	if err != nil {
		return err
	}

	totalRows := 0
	for _, count := range result.RowsInserted {
		totalRows += count
	}

	fmt.Printf("Applied %d table(s), %d row(s) total\n", len(result.TablesApplied), totalRows)
	return nil
}
