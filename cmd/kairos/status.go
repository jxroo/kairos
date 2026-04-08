package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/jxroo/kairos/internal/config"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the Kairos daemon is running",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	dataDir, err := config.DefaultDataDir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(dataDir)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://%s:%d/health", cfg.Server.Host, cfg.Server.Port)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Kairos is not running.")
		return nil
	}
	defer resp.Body.Close()

	var status map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return fmt.Errorf("decoding status response: %w", err)
	}
	fmt.Printf("Kairos is running: %s\n", status["status"])
	return nil
}
