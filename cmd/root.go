package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/fixturize/fixturize"
	"github.com/spf13/cobra"
)

var (
	version = "dev"

	profilePath   string
	loadedProfile *fixturize.Profile

	RootCmd = &cobra.Command{
		Use:               "fixturize",
		Short:             "Extract and apply database fixtures",
		Version:           version,
		PersistentPreRunE: loadProfile,
	}
)

func init() {
	RootCmd.PersistentFlags().StringVar(&profilePath, "profile", "", "Path to profile YAML file (default: .fixturize.yaml if exists)")
}

func loadProfile(cmd *cobra.Command, args []string) error {
	path := profilePath

	if path == "" {
		if _, err := os.Stat(".fixturize.yaml"); err == nil {
			path = ".fixturize.yaml"
		}
	}

	if path == "" {
		return nil
	}

	profile, err := fixturize.LoadProfile(path)
	if err != nil {
		return fmt.Errorf("failed to load profile %s: %w", path, err)
	}

	loadedProfile = profile
	return nil
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// resolveConnAndSchema resolves connection and schema from flags, profile, and env.
func resolveConnAndSchema(cmd *cobra.Command, flagConn, flagSchema string) (conn, schema string, err error) {
	conn = flagConn
	schema = flagSchema

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
		return "", "", fmt.Errorf("connection string is required (use --connection, DATABASE_URL env var, or profile)")
	}

	conn = expandEnvVars(conn)
	return conn, schema, nil
}
