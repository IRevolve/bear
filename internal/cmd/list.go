package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
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
	lockFile, err := config.LoadLock(filepath.Join(rootPath, "bear.lock.yml"))
	if err != nil {
		return fmt.Errorf("error loading lock: %w", err)
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
			if status := getStatus(p, a, lockFile); status != "" {
				p.Detail("Deployed:", status)
			}
		}

		if len(a.Artifact.Vars) > 0 {
			p.Detail("Vars:    ", "")
			keys := make([]string, 0, len(a.Artifact.Vars))
			for k := range a.Artifact.Vars {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				p.Printf("               %s\n", p.dim(fmt.Sprintf("%s: %s", k, a.Artifact.Vars[k])))
			}
		}

		if len(a.Artifact.Depends) > 0 {
			p.Detail("Depends: ", strings.Join(a.Artifact.Depends, ", "))
		}

		p.Blank()
	}

	return nil
}
