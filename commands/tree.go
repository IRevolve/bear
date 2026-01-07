package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "tree [artifacts...]",
		Short: "Display dependency tree",
		Long: `Renders a visual dependency tree for artifacts.

Shows which artifacts depend on which, with transitive dependencies.
Libraries are marked with 📚, services with 📦.

Examples:
  bear tree                   # Show full dependency tree
  bear tree user-api          # Show tree for specific artifact
  bear tree -d ./project      # Tree for different directory`,
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

			return cmd.Tree(configPath, args)
		},
	})
}
