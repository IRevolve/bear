package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IRevolve/Bear/internal/cmd"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list [path]",
	Short: "List all artifacts with their language and configuration",
	Long: `List all discovered artifacts in the workspace.
Shows each artifact's name, language, target, and dependencies.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		workDir := "."
		if len(args) > 0 {
			workDir = args[0]
		}

		// Konvertiere zu absolutem Pfad
		workDir, err := filepath.Abs(workDir)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		configPath := filepath.Join(workDir, "bear.config.yml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return fmt.Errorf("config file not found: %s", configPath)
		}

		return cmd.List(configPath)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
