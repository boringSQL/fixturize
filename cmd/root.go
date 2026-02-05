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
