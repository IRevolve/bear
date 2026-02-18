package commands

import (
	"fmt"

	"github.com/irevolve/bear/internal"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags
var Version = "dev"

var (
	// Global Flags
	workDir string
	force   bool
	verbose bool
)

var rootCmd = &cobra.Command{
	Use:          "bear",
	Short:        "Bear - Build, Evaluate, Apply, Repeat",
	Version:      Version,
	SilenceUsage: true,
	Long: `Bear is a CI/CD tool for monorepos that automatically detects
changes, validates affected artifacts, and deploys them to various targets.

It uses a plan/apply workflow to give you visibility and control over
what gets deployed. Plan validates and creates a deployment plan,
apply executes it.

Change detection is based on comparing against the last deployed commit
for each artifact (stored in bear.lock.toml).

Usage:
  bear check                     Validate configuration and dependencies
  bear list                      List all artifacts
  bear list --tree               Show dependency tree
  bear plan                      Validate changes and create deployment plan
  bear apply                     Execute the deployment plan`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// PersistentPreRunE runs before every command
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Setup logger based on verbose flag
		internal.SetupLogger(verbose)
		return nil
	}

	// Global Flags
	rootCmd.PersistentFlags().StringVarP(&workDir, "dir", "d", ".", "Path to project directory")
	rootCmd.PersistentFlags().BoolVarP(&force, "force", "f", false, "Force operation, ignoring pinned artifacts")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose debug output")

	// Version template
	rootCmd.SetVersionTemplate(fmt.Sprintf("bear version %s\n", Version))
}
