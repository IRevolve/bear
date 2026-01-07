package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
)

func List(configPath string) error {
	cfg, err := internal.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Search in the directory of the config file
	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	artifacts, err := internal.ScanArtifacts(rootPath, cfg)
	if err != nil {
		return fmt.Errorf("error scanning artifacts: %w", err)
	}

	if len(artifacts) == 0 {
		fmt.Println("No artifacts found.")
		return nil
	}

	fmt.Printf("Found %d artifact(s) in %s:\n\n", len(artifacts), cfg.Name)

	for _, a := range artifacts {
		relPath, _ := filepath.Rel(rootPath, a.Path)

		if a.Artifact.IsLib {
			fmt.Printf("📚 %s (library)\n", a.Artifact.Name)
		} else {
			fmt.Printf("📦 %s\n", a.Artifact.Name)
		}
		fmt.Printf("   Path:     %s\n", relPath)
		fmt.Printf("   Language: %s\n", a.Language)

		if !a.Artifact.IsLib {
			fmt.Printf("   Target:   %s\n", a.Artifact.Target)
		}

		if len(a.Artifact.Params) > 0 {
			fmt.Printf("   Params:\n")
			for k, v := range a.Artifact.Params {
				fmt.Printf("     %s: %s\n", k, v)
			}
		}

		if len(a.Artifact.DependsOn) > 0 {
			fmt.Printf("   Depends:  %s\n", strings.Join(a.Artifact.DependsOn, ", "))
		}

		fmt.Println()
	}

	return nil
}
