package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
)

// ValidationResult contains the result of a validation
type ValidationResult struct {
	Errors   []string
	Warnings []string
}

func (v *ValidationResult) AddError(format string, args ...interface{}) {
	v.Errors = append(v.Errors, fmt.Sprintf(format, args...))
}

func (v *ValidationResult) AddWarning(format string, args ...interface{}) {
	v.Warnings = append(v.Warnings, fmt.Sprintf(format, args...))
}

func (v *ValidationResult) HasErrors() bool {
	return len(v.Errors) > 0
}

func Check(configPath string) error {
	result := &ValidationResult{}
	p := NewPrinter()

	p.BearHeader("Check")

	checkSteps := []string{
		"Loading config",
		"Checking languages",
		"Checking targets",
		"Scanning artifacts",
		"Checking dependencies",
		"Checking for cycles",
	}

	pt := NewProgressTracker(p, "Validating configuration", checkSteps)
	pt.Start()

	// 1. Load config
	pt.MarkRunning(0)
	cfg, err := internal.Load(configPath)
	if err != nil {
		result.AddError("Failed to load config: %v", err)
		pt.MarkFailed(0, err, "")
		pt.Stop()
		return printCheckResult(p, result)
	}
	pt.MarkDone(0)

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	// 2. Check languages
	pt.MarkRunning(1)
	if len(cfg.Languages) == 0 {
		result.AddWarning("No languages defined")
	} else {
		for name, lang := range cfg.Languages {
			if len(lang.Detection.Files) == 0 && lang.Detection.Pattern == "" {
				result.AddWarning("Language '%s' has no detection rules", name)
			}
		}
	}
	pt.MarkDone(1)

	// 3. Check targets
	pt.MarkRunning(2)
	if len(cfg.Targets) == 0 {
		result.AddWarning("No targets defined")
	}
	targetNames := make(map[string]bool)
	for name := range cfg.Targets {
		targetNames[name] = true
	}
	pt.MarkDone(2)

	// 4. Scan artifacts
	pt.MarkRunning(3)
	artifacts, err := internal.ScanArtifacts(rootPath, cfg)
	if err != nil {
		result.AddError("Failed to scan artifacts: %v", err)
		pt.MarkFailed(3, err, "")
		pt.Stop()
		return printCheckResult(p, result)
	}
	if len(artifacts) == 0 {
		result.AddWarning("No artifacts found")
	}
	pt.MarkDone(3)

	// 5. Create artifact map and check dependencies
	pt.MarkRunning(4)
	artifactMap := make(map[string]internal.DiscoveredArtifact)
	for _, a := range artifacts {
		if existing, ok := artifactMap[a.Artifact.Name]; ok {
			result.AddError("Duplicate artifact name '%s' in:\n       - %s\n       - %s",
				a.Artifact.Name, existing.Path, a.Path)
		}
		artifactMap[a.Artifact.Name] = a
	}

	depErrors := 0
	for _, a := range artifacts {
		if a.Language == "unknown" {
			result.AddWarning("Artifact '%s' has unknown language", a.Artifact.Name)
		}

		if !a.Artifact.IsLib {
			if a.Artifact.Target == "" {
				result.AddError("Artifact '%s' has no target defined", a.Artifact.Name)
			} else if !targetNames[a.Artifact.Target] {
				result.AddError("Artifact '%s' references unknown target '%s'",
					a.Artifact.Name, a.Artifact.Target)
			}
		}

		for _, dep := range a.Artifact.Depends {
			if _, ok := artifactMap[dep]; !ok {
				result.AddError("Artifact '%s' depends on unknown artifact '%s'",
					a.Artifact.Name, dep)
				depErrors++
			}
		}
	}
	if depErrors > 0 {
		pt.MarkFailed(4, fmt.Errorf("%d unresolved dependencies", depErrors), "")
	} else {
		pt.MarkDone(4)
	}

	// 6. Check for circular dependencies
	pt.MarkRunning(5)
	cycles := findCycles(artifacts)
	if len(cycles) > 0 {
		for _, cycle := range cycles {
			result.AddError("Circular dependency: %s", strings.Join(cycle, " → "))
		}
		pt.MarkFailed(5, fmt.Errorf("circular dependencies found"), "")
	} else {
		pt.MarkDone(5)
	}

	pt.Stop()
	p.Blank()
	return printCheckResult(p, result)
}

// findCycles finds circular dependencies
func findCycles(artifacts []internal.DiscoveredArtifact) [][]string {
	var cycles [][]string

	// Build adjacency map
	deps := make(map[string][]string)
	for _, a := range artifacts {
		deps[a.Artifact.Name] = a.Artifact.Depends
	}

	// DFS for each node
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	var path []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, dep := range deps[node] {
			if !visited[dep] {
				if dfs(dep) {
					return true
				}
			} else if recStack[dep] {
				// Cycle found - extract the cycle path
				cycleStart := -1
				for i, n := range path {
					if n == dep {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := append(path[cycleStart:], dep)
					cycles = append(cycles, cycle)
				}
				return true
			}
		}

		path = path[:len(path)-1]
		recStack[node] = false
		return false
	}

	for _, a := range artifacts {
		if !visited[a.Artifact.Name] {
			path = []string{}
			dfs(a.Artifact.Name)
		}
	}

	return cycles
}

func printCheckResult(p *Printer, result *ValidationResult) error {
	if len(result.Warnings) > 0 {
		p.Printf("  %s\n", p.yellow("Warnings:"))
		for _, w := range result.Warnings {
			p.Printf("    %s %s\n", p.yellow("•"), w)
		}
		p.Blank()
	}

	if result.HasErrors() {
		p.Printf("  %s\n", p.red("Errors:"))
		for _, e := range result.Errors {
			p.Printf("    %s %s\n", p.red("•"), e)
		}
		p.Blank()
		p.Printf("  %s\n", p.red(fmt.Sprintf("Check failed with %d error(s)", len(result.Errors))))
		return fmt.Errorf("validation failed")
	}

	p.Printf("  %s\n", p.green("All checks passed!"))
	return nil
}
