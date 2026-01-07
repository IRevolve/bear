package commands

import (
	"fmt"
	"sort"

	"github.com/irevolve/bear/internal"
	"github.com/spf13/cobra"
)

var presetCmd = &cobra.Command{
	Use:   "preset",
	Short: "Manage presets",
	Long: `Manage language and target presets.

Presets are fetched from https://github.com/irevolve/bear-presets
and cached locally in ~/.bear/presets/

Commands:
  bear preset list     List all available presets
  bear preset update   Update local preset cache`,
}

var presetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available presets",
	RunE: func(c *cobra.Command, args []string) error {
		manager := internal.NewManager()

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
		fmt.Println("    languages: [go, node]")
		fmt.Println("    targets: [docker, cloudrun]")

		return nil
	},
}

var presetUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update local preset cache",
	RunE: func(c *cobra.Command, args []string) error {
		manager := internal.NewManager()

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
		manager := internal.NewManager()

		fmt.Println()

		switch presetType {
		case "language", "lang", "l":
			lang, err := manager.GetLanguage(name)
			if err != nil {
				return fmt.Errorf("unknown language: %s (run 'bear preset update' to refresh cache)", name)
			}

			fmt.Printf("📝 Language: %s\n", lang.Name)
			fmt.Println("───────────────────────")
			fmt.Println()
			fmt.Println("Detection:")
			if len(lang.Detection.Files) > 0 {
				fmt.Printf("  files: %v\n", lang.Detection.Files)
			}
			if lang.Detection.Pattern != "" {
				fmt.Printf("  pattern: %s\n", lang.Detection.Pattern)
			}
			fmt.Println()
			fmt.Println("Validation:")
			if len(lang.Validation.Setup) > 0 {
				fmt.Println("  setup:")
				for _, s := range lang.Validation.Setup {
					fmt.Printf("    - %s: %s\n", s.Name, s.Run)
				}
			}
			if len(lang.Validation.Lint) > 0 {
				fmt.Println("  lint:")
				for _, s := range lang.Validation.Lint {
					fmt.Printf("    - %s: %s\n", s.Name, s.Run)
				}
			}
			if len(lang.Validation.Test) > 0 {
				fmt.Println("  test:")
				for _, s := range lang.Validation.Test {
					fmt.Printf("    - %s: %s\n", s.Name, s.Run)
				}
			}
			if len(lang.Validation.Build) > 0 {
				fmt.Println("  build:")
				for _, s := range lang.Validation.Build {
					fmt.Printf("    - %s: %s\n", s.Name, s.Run)
				}
			}

		case "target", "t":
			target, err := manager.GetTarget(name)
			if err != nil {
				return fmt.Errorf("unknown target: %s (run 'bear preset update' to refresh cache)", name)
			}

			fmt.Printf("🎯 Target: %s\n", target.Name)
			fmt.Println("───────────────────────")
			fmt.Println()
			if len(target.Defaults) > 0 {
				fmt.Println("Defaults:")
				for k, v := range target.Defaults {
					fmt.Printf("  %s: %s\n", k, v)
				}
				fmt.Println()
			}
			fmt.Println("Deploy:")
			for _, s := range target.Deploy {
				fmt.Printf("  - %s: %s\n", s.Name, s.Run)
			}

		default:
			return fmt.Errorf("unknown preset type: %s (use 'language' or 'target')", presetType)
		}

		return nil
	},
}

func init() {
	presetCmd.AddCommand(presetListCmd)
	presetCmd.AddCommand(presetUpdateCmd)
	presetCmd.AddCommand(presetShowCmd)
	rootCmd.AddCommand(presetCmd)
}
