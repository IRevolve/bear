package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [artifacts...]",
	Short: "Show what would be validated/deployed based on changes",
	Long: `Creates an execution plan showing which artifacts have changed
and what actions would be taken (validate, deploy, skip).

The plan compares each artifact against its last deployed commit
(from bear.lock.yml) to determine what needs to be built and deployed.

Examples:
  bear plan                      # Plan all changed artifacts
  bear plan user-api             # Plan specific artifact
  bear plan user-api order-api   # Plan multiple artifacts
  bear plan -d ./other-project   # Plan in different directory`,
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

		opts := cmd.Options{
			Artifacts:      args, // Positional args are the artifacts
			RollbackCommit: rollback,
			DryRun:         dryRun,
			Force:          force,
		}

		return cmd.PlanWithOptions(configPath, opts)
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
