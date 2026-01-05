package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IRevolve/Bear/internal/cmd"
	"github.com/spf13/cobra"
)

var commitLock bool

var applyCmd = &cobra.Command{
	Use:   "apply [artifacts...]",
	Short: "Execute the plan (validate and deploy changed artifacts)",
	Long: `Executes the deployment plan in two phases:

Phase 1: Validation
  Runs setup, lint, test, and build steps for all changed artifacts.

Phase 2: Deployment
  Deploys validated artifacts to their configured targets.

After successful deployment, the lock file is updated with the
deployed commit hash.

Examples:
  bear apply                           # Apply all changed artifacts
  bear apply user-api                  # Apply specific artifact
  bear apply user-api --rollback=abc   # Rollback to commit (pins artifact)
  bear apply user-api --force          # Force apply, remove pin
  bear apply --commit                  # Apply and commit lock file with [skip ci]`,
	RunE: func(c *cobra.Command, args []string) error {
		// Konvertiere zu absolutem Pfad
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
			Commit:         commitLock,
		}

		return cmd.ApplyWithOptions(configPath, opts)
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVarP(&commitLock, "commit", "c", false, "Commit and push lock file with [skip ci]")
}
