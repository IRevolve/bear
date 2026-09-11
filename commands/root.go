package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
changes and deploys them to various targets.

It uses a plan/apply workflow to give you visibility and control over
what gets deployed. Plan is a fast, side-effect-free decision: it detects
changes and writes a deployment plan without running anything. Apply is
the only command that executes: for every deploying artifact it builds
(the language's steps) and then deploys (the target's steps).

Run 'bear validate' for build/test assurance before merging or planning;
it runs the same language steps as apply's build phase, with no
environment or deployment semantics.

Change detection is based on comparing against the last deployed commit
for each artifact (stored in bear.lock.yml).

Deployment environments are declared per project in bear.config.yml.

Usage:
  bear doctor                    Diagnose configuration and dependencies
  bear validate                  Run language build/test steps (no deployment)
  bear list                      List all artifacts
  bear list --tree               Show dependency tree
  bear plan <environment>        Detect changes and create a deployment plan
  bear apply                     Build and deploy the plan`,
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	// Global Flags
	rootCmd.PersistentFlags().StringVarP(&workDir, "dir", "d", ".", "Path to project directory")
	rootCmd.PersistentFlags().BoolVarP(&force, "force", "f", false, "Force operation, ignoring pinned artifacts")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose debug output")

	// Version template
	rootCmd.SetVersionTemplate(fmt.Sprintf("bear version %s\n", Version))
}
