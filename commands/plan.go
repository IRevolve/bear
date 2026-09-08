package commands

import (
	"fmt"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var (
	planConcurrency int
	planPinCommit   string
)

var planCmd = &cobra.Command{
	Use:   "plan <environment> [artifacts...]",
	Short: "Detect changes, validate artifacts, and show the deployment plan",
	Long: `Detects changed artifacts, runs validation steps in parallel,
and writes a validated deployment plan to .bear/plan.yml.

The plan compares each artifact against its last deployed commit
(from bear.lock.yml) and validates all changed artifacts before
showing what would be deployed.

If validation fails, no plan file is written and the command exits with code 1.
After argument parsing, planning removes the previous saved plan first.
Deployment requires explicit membership in each artifact's environments allowlist.
Missing or empty environments disables deployment, but validation still runs.
The first argument must be dev, int, or prd, including for validation-only plans.
Job variables do not select the deployment environment.

After a successful plan, run 'bear apply' to execute the deployments.

Examples:
  bear plan dev                    # Plan all changed artifacts
  bear plan dev user-api           # Plan specific artifact
  bear plan int user-api order-api # Plan multiple artifacts
  bear plan prd --pin abc123       # Pin artifact(s) to specific commit
  bear plan dev --concurrency 5    # Limit parallel validations
  bear plan int -d ./other-project # Plan in different directory`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		// Convert to absolute path
		absDir, err := filepath.Abs(workDir)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		configPath := filepath.Join(absDir, "bear.config.yml")

		opts := cmd.Options{
			Context:     c.Context(),
			Environment: args[0],
			Artifacts:   args[1:],
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
