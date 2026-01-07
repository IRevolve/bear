package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags
var Version = "dev"

var (
	// Globale Flags
	workDir string
	force   bool
)

var rootCmd = &cobra.Command{
	Use:     "bear",
	Short:   "Bear - Build, Evaluate, Apply, Repeat",
	Version: Version,
	Long: `Bear is a CI/CD tool for monorepos that automatically detects
changes, validates affected artifacts, and deploys them to various targets.

It uses a Terraform-like plan/apply workflow to give you visibility
and control over what gets deployed.

Change detection is based on comparing against the last deployed commit
for each artifact (stored in bear.lock.yml).

Usage:
  bear list                      List all artifacts
  bear list --tree               Show dependency tree
  bear plan                      Plan changes for artifacts
  bear plan --validate           Plan and run validation
  bear apply                     Apply changes to artifacts`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Globale Flags
	rootCmd.PersistentFlags().StringVarP(&workDir, "dir", "d", ".", "Path to project directory")
	rootCmd.PersistentFlags().BoolVarP(&force, "force", "f", false, "Force operation, ignoring pinned artifacts")

	// Version template
	rootCmd.SetVersionTemplate(fmt.Sprintf("bear version %s\n", Version))
}
