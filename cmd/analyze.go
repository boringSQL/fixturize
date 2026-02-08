package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	analyzeCmd = &cobra.Command{
		Use:   "analyze",
		Short: "Detect PII columns and suggest masks",
		Long: `Scan a PostgreSQL schema for columns that likely contain personally
identifiable information (PII) based on column names and types.

Outputs a report with suggested mask expressions ready for use with extract --mask.

Examples:
  # Analyze all tables
  fixturize analyze --connection "$DB"

  # Analyze tables reachable from users
  fixturize analyze --connection "$DB" --root users --depth 2

  # Only show medium+ confidence detections
  fixturize analyze --connection "$DB" --min-confidence medium`,
		RunE: runAnalyze,
	}

	analyzeConn          string
	analyzeSchema        string
	analyzeRoot          string
	analyzeDepth         int
	analyzeMinConfidence string
)

func init() {
	RootCmd.AddCommand(analyzeCmd)

	analyzeCmd.Flags().StringVar(&analyzeConn, "connection", "", "PostgreSQL connection string")
	analyzeCmd.Flags().StringVar(&analyzeSchema, "schema", "", "Default schema for unqualified names (default: public)")
	analyzeCmd.Flags().StringVar(&analyzeRoot, "root", "", "Filter to tables reachable from this root table")
	analyzeCmd.Flags().IntVar(&analyzeDepth, "depth", 0, "Max FK hops from root (0 = unlimited, only with --root)")
	analyzeCmd.Flags().StringVar(&analyzeMinConfidence, "min-confidence", "low", "Minimum confidence level: low, medium, high")
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	conn := analyzeConn
	schema := analyzeSchema
	root := analyzeRoot
	depth := analyzeDepth

	if loadedProfile != nil {
		if !cmd.Flags().Changed("connection") && conn == "" {
			conn = loadedProfile.Connection
		}
		if !cmd.Flags().Changed("schema") && schema == "" {
			schema = loadedProfile.Schema
		}
	}
	if schema == "" {
		schema = "public"
	}

	if conn == "" {
		conn = os.Getenv("DATABASE_URL")
	}
	if conn == "" {
		return fmt.Errorf("connection string is required (use --connection, DATABASE_URL env var, or profile)")
	}

	minConf, err := fixturize.ParseConfidence(analyzeMinConfidence)
	if err != nil {
		return err
	}

	conn = expandEnvVars(conn)
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

	result := fixturize.AnalyzeSchema(dbSchema, tables, minConf)
	fmt.Print(fixturize.FormatAnalysis(dbSchema, result))
	return nil
}
