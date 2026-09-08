package commands

import (
	"fmt"
	"sort"

	"github.com/irevolve/bear/internal"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var presetRevision string

var presetCmd = &cobra.Command{
	Use:   "preset",
	Short: "Manage presets",
	Long: `Manage language and target presets.

Presets are fetched from https://github.com/irevolve/bear-presets
at an immutable revision and cached locally in ~/.bear/presets/<revision>/.
Use --revision to select a different commit SHA.

Commands:
  bear preset list     List all available presets
  bear preset update   Update local preset cache`,
}

var presetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available presets",
	RunE: func(c *cobra.Command, args []string) error {
		manager := internal.NewManager(presetRevision)

		fmt.Println()
		fmt.Println("📦 Available Presets")
		fmt.Println("====================")

		index, err := manager.GetIndex()
		if err != nil {
			return fmt.Errorf("could not fetch presets: %w\n\nRun 'bear preset update' to refresh cache", err)
		}

		fmt.Println()
		fmt.Println("Languages:")
		sort.Strings(index.Languages)
		for _, l := range index.Languages {
			fmt.Printf("  • %s\n", l)
		}

		fmt.Println()
		fmt.Println("Targets:")
		sort.Strings(index.Targets)
		for _, t := range index.Targets {
			fmt.Printf("  • %s\n", t)
		}

		fmt.Println()
		fmt.Println("Usage in bear.config.yml:")
		fmt.Println("  use:")
		fmt.Printf("    revision: %s\n", presetRevision)
		fmt.Println("    languages: [go, node]")
		fmt.Println("    targets: [docker, cloudrun]")

		return nil
	},
}

var presetUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update local preset cache",
	RunE: func(c *cobra.Command, args []string) error {
		manager := internal.NewManager(presetRevision)

		fmt.Println("🔄 Updating presets from GitHub...")

		if err := manager.Update(); err != nil {
			return fmt.Errorf("failed to update presets: %w", err)
		}

		fmt.Println("✅ Presets updated successfully!")
		return nil
	},
}

var presetShowCmd = &cobra.Command{
	Use:   "show <type> <name>",
	Short: "Show details of a preset",
	Long: `Show the full configuration of a preset.

Examples:
  bear preset show language go
  bear preset show target cloudrun`,
	Args: cobra.ExactArgs(2),
	RunE: func(c *cobra.Command, args []string) error {
		presetType := args[0]
		name := args[1]
		manager := internal.NewManager(presetRevision)
		encoder := yaml.NewEncoder(c.OutOrStdout())
		encoder.SetIndent(2)
		defer encoder.Close()

		switch presetType {
		case "language", "lang", "l":
			lang, err := manager.GetLanguage(name)
			if err != nil {
				return fmt.Errorf("language preset %q: %w", name, err)
			}

			return encoder.Encode(lang)

		case "target", "t":
			target, err := manager.GetTarget(name)
			if err != nil {
				return fmt.Errorf("target preset %q: %w", name, err)
			}

			return encoder.Encode(target)

		default:
			return fmt.Errorf("unknown preset type: %s (use 'language' or 'target')", presetType)
		}

	},
}

func init() {
	presetCmd.PersistentFlags().StringVar(&presetRevision, "revision", internal.DefaultPresetsRevision, "Immutable preset repository commit SHA")
	presetCmd.AddCommand(presetListCmd)
	presetCmd.AddCommand(presetUpdateCmd)
	presetCmd.AddCommand(presetShowCmd)
	rootCmd.AddCommand(presetCmd)
}
