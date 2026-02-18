package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var (
	planConcurrency int
	planPinCommit   string
)

var planCmd = &cobra.Command{
	Use:   "plan [artifacts...]",
	Short: "Detect changes, validate artifacts, and show the deployment plan",
	Long: `Detects changed artifacts, runs validation steps in parallel,
and writes a validated deployment plan to .bear/plan.yml.

The plan compares each artifact against its last deployed commit
(from bear.lock.yml) and validates all changed artifacts before
showing what would be deployed.

If validation fails, no plan file is written and the command exits with code 1.

After a successful plan, run 'bear apply' to execute the deployments.

Examples:
  bear plan                        # Plan all changed artifacts
  bear plan user-api               # Plan specific artifact
  bear plan user-api order-api     # Plan multiple artifacts
  bear plan --pin abc123           # Pin artifact(s) to specific commit
  bear plan --concurrency 5        # Limit parallel validations
  bear plan -d ./other-project     # Plan in different directory`,
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
			Artifacts:   args,
			PinCommit:   planPinCommit,
			Force:       force,
			Concurrency: planConcurrency,
			Verbose:     verbose,
		}

		return cmd.PlanWithOptions(configPath, opts)
	},
}

func init() {
	planCmd.Flags().IntVar(&planConcurrency, "concurrency", 10, "Maximum number of parallel validation jobs")
	planCmd.Flags().StringVar(&planPinCommit, "pin", "", "Pin artifact(s) to a specific commit")
	rootCmd.AddCommand(planCmd)
}
