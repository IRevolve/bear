package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all artifacts with their language and configuration",
	Long: `List all discovered artifacts in the workspace.
Shows each artifact's name, language, target, and dependencies.

Examples:
  bear list                # List all artifacts
  bear list -d ./project   # List artifacts in different directory`,
	RunE: func(c *cobra.Command, args []string) error {
		// Convert to absolute path
		absDir, err := filepath.Abs(workDir)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		configPath := filepath.Join(absDir, "bear.config.yml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return fmt.Errorf("config file not found: %s", configPath)
		}

		return cmd.List(configPath)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
