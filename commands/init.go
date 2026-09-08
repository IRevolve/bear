package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
language and target presets. With no flags, creates an empty configuration;
presets are not detected automatically and no interactive prompts are shown.

Available language presets:
  go, node, typescript, python, rust, java

Available target presets:
  docker, cloudrun, cloudrun-job, lambda, s3, s3-static,
  kubernetes, helm

Examples:
  bear init                           # Empty configuration
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
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("checking config: %w", err)
		}

		// Use folder name as project name
		projectName := filepath.Base(absDir)
		if strings.TrimSpace(projectName) == "" {
			return fmt.Errorf("project directory name must not be blank")
		}

		manager := internal.NewManager()

		// Validate languages
		for _, lang := range initLanguages {
			if _, err := manager.GetLanguage(lang); err != nil {
				return fmt.Errorf("language preset %q: %w", lang, err)
			}
		}

		// Validate targets
		for _, target := range initTargets {
			if _, err := manager.GetTarget(target); err != nil {
				return fmt.Errorf("target preset %q: %w", target, err)
			}
		}

		// Generate config
		data, err := generateConfig(projectName, initLanguages, initTargets)
		if err != nil {
			return err
		}

		// Write file
		flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
		if initForce {
			flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		file, err := os.OpenFile(configPath, flags, 0644)
		if err != nil {
			return fmt.Errorf("failed to create config: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			return fmt.Errorf("failed to write config: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("failed to close config: %w", err)
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
		fmt.Println("  4. Run 'bear plan <environment>' to validate and plan deployments (dev, int, or prd)")
		fmt.Println("  5. Run 'bear apply' to execute the plan")

		return nil
	},
}

func generateConfig(name string, languages, targets []string) ([]byte, error) {
	var sb strings.Builder
	cfg := config.Config{Name: name}
	if len(languages) > 0 || len(targets) > 0 {
		cfg.Use = config.UseConfig{Revision: internal.DefaultPresetsRevision, Languages: languages, Targets: targets}
	}
	encoder := yaml.NewEncoder(&sb)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		return nil, fmt.Errorf("encode project config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, err
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

	return []byte(sb.String()), nil
}

func init() {
	initCmd.Flags().StringSliceVar(&initLanguages, "lang", nil, "Language presets to use (go,node,python,rust,java,typescript)")
	initCmd.Flags().StringSliceVar(&initTargets, "target", nil, "Target presets to use (docker,cloudrun,lambda,s3,kubernetes,...)")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing config")
	rootCmd.AddCommand(initCmd)
}
