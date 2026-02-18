package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var (
	applyNoCommit    bool
	applyConcurrency int
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Execute the deployment plan",
	Long: `Reads the plan from .bear/plan.yml (created by 'bear plan') and
executes the deployments in parallel.

After successful deployment, the lock file is updated and automatically
committed with [skip ci]. Use --no-commit to disable auto-commit.

The plan file is removed after execution.

Requires a plan file — run 'bear plan' first.

Examples:
  bear plan && bear apply          # Plan and apply
  bear apply                       # Apply existing plan
  bear apply --no-commit           # Apply without committing lock file
  bear apply --concurrency 5       # Limit parallel deployments`,
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
			Force:       force,
			NoCommit:    applyNoCommit,
			Concurrency: applyConcurrency,
			Verbose:     verbose,
		}

		return cmd.ApplyWithOptions(configPath, opts)
	},
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().BoolVar(&applyNoCommit, "no-commit", false, "Do not commit and push lock file after deployment")
	applyCmd.Flags().IntVar(&applyConcurrency, "concurrency", 10, "Maximum number of parallel deployment jobs")
}
