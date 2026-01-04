package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/IRevolve/Bear/internal/config"
	"github.com/spf13/cobra"
)

var unpinCmd = &cobra.Command{
	Use:   "unpin <artifact> [artifact...]",
	Short: "Unpin artifacts to allow automatic updates again",
	Long: `Removes the pin from one or more artifacts.

Pinned artifacts are locked to a specific version (typically after a rollback)
and won't be updated by 'bear apply' until unpinned.

Examples:
  bear unpin user-api
  bear unpin user-api order-api
  bear unpin --all`,
	Args: cobra.MinimumNArgs(0),
	RunE: func(c *cobra.Command, args []string) error {
		workDir := "."

		// Konvertiere zu absolutem Pfad
		workDir, err := filepath.Abs(workDir)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		lockPath := filepath.Join(workDir, "bear.lock.yml")
		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			return fmt.Errorf("lock file not found: %s", lockPath)
		}

		lockFile, err := config.LoadLock(lockPath)
		if err != nil {
			return fmt.Errorf("error loading lock file: %w", err)
		}

		unpinAll, _ := c.Flags().GetBool("all")

		if unpinAll {
			count := 0
			for name, entry := range lockFile.Artifacts {
				if entry.Pinned {
					lockFile.UnpinArtifact(name)
					fmt.Printf("📌 Unpinned: %s\n", name)
					count++
				}
			}
			if count == 0 {
				fmt.Println("No pinned artifacts found.")
				return nil
			}
		} else {
			if len(args) == 0 {
				return fmt.Errorf("specify artifact names or use --all")
			}

			for _, name := range args {
				if lockFile.IsPinned(name) {
					lockFile.UnpinArtifact(name)
					fmt.Printf("📌 Unpinned: %s\n", name)
				} else if _, ok := lockFile.Artifacts[name]; ok {
					fmt.Printf("ℹ️  %s is not pinned\n", name)
				} else {
					fmt.Printf("⚠️  %s not found in lock file\n", name)
				}
			}
		}

		if err := lockFile.Save(lockPath); err != nil {
			return fmt.Errorf("error saving lock file: %w", err)
		}

		fmt.Println("\n✅ Lock file updated")
		return nil
	},
}

func init() {
	unpinCmd.Flags().Bool("all", false, "Unpin all pinned artifacts")
	rootCmd.AddCommand(unpinCmd)
}
