package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	buildversion "github.com/jxroo/kairos/internal/version"
)

var rootCmd = &cobra.Command{
	Use:     "kairos",
	Short:   "Personal AI Runtime",
	Long:    "Kairos — a local AI platform with persistent memory, RAG, and MCP support.",
	Version: buildversion.Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
