package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/spf13/cobra"
)

var (
	initLanguages []string
	initTargets   []string
	initForce     bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Bear project",
	Long: `Creates a new bear.config.yml in the current directory.

Uses the folder name as project name and imports the specified
language and target presets.

Available language presets:
  go, node, typescript, python, rust, java

Available target presets:
  docker, cloudrun, cloudrun-job, lambda, s3, s3-static,
  kubernetes, helm, fly, vercel, netlify

Examples:
  bear init                           # Interactive
  bear init --lang go,node            # Go + Node presets
  bear init --lang go --target docker # Go + Docker
  bear init -d ./new-project          # Different directory`,
	RunE: func(c *cobra.Command, args []string) error {
		// Convert to absolute path
		absDir, err := filepath.Abs(workDir)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}

		configPath := filepath.Join(absDir, "bear.config.yml")

		// Check if already exists
		if _, err := os.Stat(configPath); err == nil && !initForce {
			return fmt.Errorf("config file already exists: %s (use --force to overwrite)", configPath)
		}

		// Use folder name as project name
		projectName := filepath.Base(absDir)

		manager := internal.NewManager()

		// Validate languages
		for _, lang := range initLanguages {
			if _, err := manager.GetLanguage(lang); err != nil {
				index, _ := manager.GetIndex()
				if index != nil {
					sort.Strings(index.Languages)
					return fmt.Errorf("unknown language: %s\nAvailable: %s", lang, strings.Join(index.Languages, ", "))
				}
				return fmt.Errorf("unknown language: %s (run 'bear preset update' to refresh cache)", lang)
			}
		}

		// Validate targets
		for _, target := range initTargets {
			if _, err := manager.GetTarget(target); err != nil {
				index, _ := manager.GetIndex()
				if index != nil {
					sort.Strings(index.Targets)
					return fmt.Errorf("unknown target: %s\nAvailable: %s", target, strings.Join(index.Targets, ", "))
				}
				return fmt.Errorf("unknown target: %s (run 'bear preset update' to refresh cache)", target)
			}
		}

		// Generate config
		config := generateConfig(projectName, initLanguages, initTargets)

		// Write file
		if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}

		// Add .bear/ to .gitignore if it exists
		gitignorePath := filepath.Join(absDir, ".gitignore")
		if _, err := os.Stat(gitignorePath); err == nil {
			data, err := os.ReadFile(gitignorePath)
			if err == nil && !strings.Contains(string(data), ".bear/") {
				f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0644)
				if err == nil {
					defer f.Close()
					content := string(data)
					if len(content) > 0 && content[len(content)-1] != '\n' {
						f.WriteString("\n")
					}
					f.WriteString(".bear/\n")
				}
			}
		}

		fmt.Printf("Created %s\n\n", configPath)
		fmt.Println("Next steps:")
		fmt.Println("  1. Add bear.artifact.yml to your services/apps")
		fmt.Println("  2. Add bear.lib.yml to your libraries")
		fmt.Println("  3. Run 'bear check' to validate your setup")
		fmt.Println("  4. Run 'bear plan' to validate and plan deployments")
		fmt.Println("  5. Run 'bear apply' to execute the plan")

		return nil
	},
}

func generateConfig(name string, languages, targets []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("name: %s\n", name))

	// Use section if presets selected
	if len(languages) > 0 || len(targets) > 0 {
		sb.WriteString("\nuse:\n")
		if len(languages) > 0 {
			sb.WriteString(fmt.Sprintf("  languages: [%s]\n", strings.Join(languages, ", ")))
		}
		if len(targets) > 0 {
			sb.WriteString(fmt.Sprintf("  targets: [%s]\n", strings.Join(targets, ", ")))
		}
	}

	// Example comments for custom extensions
	sb.WriteString(`
# Custom languages (optional, extend or override presets)
# languages:
#   custom-lang:
#     detection:
#       files: [custom.config]
#     steps:
#       - name: Build
#         run: custom-build

# Custom targets (optional, extend or override presets)
# targets:
#   custom-target:
#     vars:
#       PARAM: value
#     steps:
#       - name: Deploy
#         run: custom-deploy $PARAM
`)

	return sb.String()
}

func init() {
	initCmd.Flags().StringSliceVar(&initLanguages, "lang", nil, "Language presets to use (go,node,python,rust,java,typescript)")
	initCmd.Flags().StringSliceVar(&initTargets, "target", nil, "Target presets to use (docker,cloudrun,lambda,s3,kubernetes,...)")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing config")
	rootCmd.AddCommand(initCmd)
}
