package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var (
	validateConcurrency int
	validateEnvironment string
)

var validateCmd = &cobra.Command{
	Use:   "validate [artifacts...]",
	Short: "Run validation steps against the working tree without planning a deployment",
	Long: `Runs the validation steps of every artifact and library against the current
working tree and reports which ones pass.

Validation has no deployment semantics. It takes no environment argument,
selects no deployment policy, compares against no deployment history, detects
no changes, prints no deploy or skip sections, and neither reads nor writes
.bear/plan.yml or bear.lock.yml. It runs no Git at all, so it works in a
shallow clone or in an export that is not a repository. Use it in merge-request
CI; use 'bear plan <environment>' when you intend to deploy.

Without arguments every discovered artifact and library is validated. Naming
artifacts validates exactly those: dependencies are not added, and a name that
matches nothing is an error so a typo fails the job.

Artifacts whose language defines no validation steps are reported as
'no validation steps' and do not fail the run.

--environment only injects ENVIRONMENT into the validation steps, using the
same variable precedence as plan. It must name an environment declared in
bear.config.yml, but it enables no deployment and changes no verdict.

Examples:
  bear validate                       # Validate every artifact and library
  bear validate user-api              # Validate a single artifact
  bear validate user-api order-api    # Validate multiple artifacts
  bear validate --environment int     # Inject ENVIRONMENT=int into the steps
  bear validate --concurrency 5       # Limit parallel validation jobs
  bear validate -v                    # Stream step output while it runs
  bear validate -d ./other-project    # Validate a different directory`,
	Args: cobra.ArbitraryArgs,
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
			Context:     c.Context(),
			Environment: validateEnvironment,
			Artifacts:   args,
			Concurrency: validateConcurrency,
			Verbose:     verbose,
		}

		return cmd.ValidateWithOptions(configPath, opts)
	},
}

func init() {
	validateCmd.Flags().IntVar(&validateConcurrency, "concurrency", 10, "Maximum number of parallel validation jobs")
	validateCmd.Flags().StringVar(&validateEnvironment, "environment", "", "Inject ENVIRONMENT into the validation steps (must be declared in bear.config.yml)")
	rootCmd.AddCommand(validateCmd)
}
