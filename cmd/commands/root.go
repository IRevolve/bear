package commands

import (
	"github.com/spf13/cobra"
)

var (
	// Globale Flags
	artifacts []string
	rollback  string
	dryRun    bool
)

var rootCmd = &cobra.Command{
	Use:   "bear",
	Short: "Bear - Build, Evaluate, Apply, Repeat",
	Long: `Bear is a CI/CD tool for monorepos that automatically detects
changes, validates affected artifacts, and deploys them to various targets.

It uses a Terraform-like plan/apply workflow to give you visibility
and control over what gets deployed.

Change detection is based on comparing against the last deployed commit
for each artifact (stored in bear.lock.yml).`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Globale Flags
	rootCmd.PersistentFlags().StringSliceVarP(&artifacts, "artifact", "a", nil, "Select specific artifact(s)")
	rootCmd.PersistentFlags().StringVar(&rollback, "rollback", "", "Rollback artifact(s) to a specific commit")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Show what would be executed without running commands")
}
