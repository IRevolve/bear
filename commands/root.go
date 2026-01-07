package commands

import (
	"github.com/spf13/cobra"
)

var (
	// Globale Flags
	workDir  string
	rollback string
	dryRun   bool
	force    bool
)

var rootCmd = &cobra.Command{
	Use:   "bear",
	Short: "Bear - Build, Evaluate, Apply, Repeat",
	Long: `Bear is a CI/CD tool for monorepos that automatically detects
changes, validates affected artifacts, and deploys them to various targets.

It uses a Terraform-like plan/apply workflow to give you visibility
and control over what gets deployed.

Change detection is based on comparing against the last deployed commit
for each artifact (stored in bear.lock.yml).

Usage:
  bear list                      List all artifacts
  bear plan [artifacts...]       Plan changes for artifacts
  bear apply [artifacts...]      Apply changes to artifacts`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Globale Flags
	rootCmd.PersistentFlags().StringVarP(&workDir, "dir", "d", ".", "Path to project directory")
	rootCmd.PersistentFlags().StringVar(&rollback, "rollback", "", "Rollback artifact(s) to a specific commit")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Show what would be executed without running commands")
	rootCmd.PersistentFlags().BoolVarP(&force, "force", "f", false, "Force operation, ignoring pinned artifacts")
}
