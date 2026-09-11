package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose configuration and dependencies",
	Long: `Diagnoses the Bear configuration and checks for issues:

- Config syntax (bear.config.yml, bear.artifact.yml, bear.lib.yml)
- All dependencies exist and can be resolved
- No circular dependencies
- All referenced targets exist
- Declared environments and artifact allowlists
- Language detection works for all artifacts

Examples:
  bear doctor                  # Diagnose current directory
  bear doctor -d ./project     # Diagnose different directory`,
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

		return cmd.Doctor(configPath)
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
