package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IRevolve/Bear/internal/cmd"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [path]",
	Short: "Show what would be validated/deployed based on changes",
	Long: `Creates an execution plan showing which artifacts have changed
and what actions would be taken (validate, deploy, skip).

The plan compares against the base branch (default: main) and the
lock file to determine what needs to be built and deployed.`,
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
			BaseBranch:     baseBranch,
		}

		return cmd.PlanWithOptions(configPath, opts)
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
