package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"

	RootCmd = &cobra.Command{
		Use:     "fixturize",
		Short:   "Extract and apply database fixtures",
		Version: version,
	}
)

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
