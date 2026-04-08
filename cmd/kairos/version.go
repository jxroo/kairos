package main

import (
	"fmt"

	"github.com/spf13/cobra"

	buildversion "github.com/jxroo/kairos/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Kairos version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(cmd.OutOrStdout(), buildversion.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
