package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
)

func List(configPath string) error {
	p := NewPrinter()

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
		p.Println("No artifacts found.")
		return nil
	}

	p.BearHeader(fmt.Sprintf("List (%d artifacts in %s)", len(artifacts), cfg.Name))

	for _, a := range artifacts {
		relPath, _ := filepath.Rel(rootPath, a.Path)

		if a.Artifact.IsLib {
			p.Printf("  %s %s\n", p.dim("lib"), p.bold(a.Artifact.Name))
		} else {
			p.Printf("  %s %s\n", p.cyan("svc"), p.bold(a.Artifact.Name))
		}
		p.Detail("Path:    ", relPath)
		p.Detail("Language:", a.Language)

		if !a.Artifact.IsLib {
			p.Detail("Target:  ", a.Artifact.Target)
		}

		if len(a.Artifact.Params) > 0 {
			p.Detail("Params:  ", "")
			for k, v := range a.Artifact.Params {
				p.Printf("               %s\n", p.dim(fmt.Sprintf("%s: %s", k, v)))
			}
		}

		if len(a.Artifact.DependsOn) > 0 {
			p.Detail("Depends: ", strings.Join(a.Artifact.DependsOn, ", "))
		}

		p.Blank()
	}

	return nil
}
