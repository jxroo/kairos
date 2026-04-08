package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Kairos daemon",
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	fmt.Println("Send SIGTERM to the running daemon process, or press Ctrl+C in the daemon terminal.")
	return nil
}
