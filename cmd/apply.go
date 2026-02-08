package cmd

import (
	"fmt"

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
	applySyncSequences   bool
)

func init() {
	RootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringVar(&applyConn, "connection", "", "PostgreSQL connection string")
	applyCmd.Flags().BoolVar(&applyForce, "force", false, "Truncate tables before applying fixture")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Show what would be done without making changes")
	applyCmd.Flags().BoolVar(&applyDisableTriggers, "disable-triggers", false, "Disable triggers during insert (uses replica mode)")
	applyCmd.Flags().BoolVar(&applySyncSequences, "sync-sequences", false, "Sync sequences to max ID after loading")
}

func runApply(cmd *cobra.Command, args []string) error {
	conn, _, err := resolveConnAndSchema(cmd, applyConn, "")
	if err != nil {
		return err
	}
	force, disableTriggers, syncSequences := mergeApplyFlags(cmd)

	db, err := fixturize.OpenDB(conn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	options := &fixturize.ApplyOptions{
		Connection:      conn,
		Fixture:         args[0],
		Force:           force,
		DryRun:          applyDryRun,
		DisableTriggers: disableTriggers,
		SyncSequences:   syncSequences,
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

func mergeApplyFlags(cmd *cobra.Command) (force, disableTriggers, syncSequences bool) {
	force = applyForce
	disableTriggers = applyDisableTriggers
	syncSequences = applySyncSequences

	if loadedProfile == nil {
		return
	}

	p := loadedProfile
	if !cmd.Flags().Changed("force") && !force {
		force = p.Apply.Force
	}
	if !cmd.Flags().Changed("disable-triggers") && !disableTriggers {
		disableTriggers = p.Apply.DisableTriggers
	}
	if !cmd.Flags().Changed("sync-sequences") && !syncSequences {
		syncSequences = p.Apply.SyncSequences
	}

	return
}
