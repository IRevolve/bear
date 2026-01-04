package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IRevolve/Bear/internal/cmd"
	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply [path]",
	Short: "Execute the plan (validate and deploy changed artifacts)",
	Long: `Executes the deployment plan in two phases:

Phase 1: Validation
  Runs setup, lint, test, and build steps for all changed artifacts.

Phase 2: Deployment
  Deploys validated artifacts to their configured targets.

After successful deployment, the lock file is updated with the
deployed commit hash.`,
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

		opts := cmd.Options{
			Artifacts:      artifacts,
			RollbackCommit: rollback,
			DryRun:         dryRun,
		}

		return cmd.ApplyWithOptions(configPath, opts)
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
}
