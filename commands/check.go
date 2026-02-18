package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate configuration and dependencies",
	Long: `Validates the Bear configuration and checks for issues:

- Config syntax (bear.config.yml, bear.artifact.yml, bear.lib.yml)
- All dependencies exist and can be resolved
- No circular dependencies
- All referenced targets exist
- Language detection works for all artifacts

Examples:
  bear check                  # Check current directory
  bear check -d ./project     # Check different directory`,
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

		return cmd.Check(configPath)
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
