package commands

import (
	"fmt"
	"path/filepath"

	"github.com/irevolve/bear/internal/cmd"
	"github.com/spf13/cobra"
)

var (
	planPinCommit string
)

var planCmd = &cobra.Command{
	Use:   "plan <environment> [artifacts...]",
	Short: "Detect changes and write a deployment plan",
	Long: `Detects changed artifacts and writes a deployment plan to .bear/plan.yml.
Plan runs no commands: it only compares against the last deployed commit
(from bear.lock.yml) and decides what would be deployed. Run 'bear validate'
first for build/test assurance; 'bear apply' is what actually builds and
deploys, using the same language steps validate runs plus the target's
deploy steps.

After argument parsing, planning removes the previous saved plan first.
Deployment requires explicit membership in each artifact's environments allowlist.
Missing or empty environments disables deployment; change detection still runs.
The first argument must be an environment declared in bear.config.yml, including
for plans with nothing to deploy. Run 'bear doctor' to see the declared environments.
Job variables do not select the deployment environment.

Deploying any artifact requires a clean Git source; commit or remove changes first.
--pin resolves the given commit and plans it without validating it — run
'bear validate' against that revision first if you want that assurance.

After a successful plan, run 'bear apply' to build and deploy it.

Examples (for a project declaring dev, int and prd):
  bear plan dev                    # Plan all changed artifacts
  bear plan dev user-api           # Plan specific artifact
  bear plan int user-api order-api # Plan multiple artifacts
  bear plan prd --pin abc123       # Pin artifact(s) to a specific commit
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
		}

		return cmd.PlanWithOptions(configPath, opts)
	},
}

func init() {
	planCmd.Flags().StringVar(&planPinCommit, "pin", "", "Pin artifact(s) to a specific commit")
	rootCmd.AddCommand(planCmd)
}
