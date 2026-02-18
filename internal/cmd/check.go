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

	// 1. Load config
	p.Printf("  Loading config... ")
	cfg, err := internal.Load(configPath)
	if err != nil {
		p.Println(p.red("✗"))
		result.AddError("Failed to load config: %v", err)
		return printCheckResult(p, result)
	}
	p.Printf("%s %s\n", p.green("✓"), cfg.Name)

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	// 2. Check languages
	p.Printf("  Checking languages... ")
	if len(cfg.Languages) == 0 {
		result.AddWarning("No languages defined")
		p.Println(p.yellow("none defined"))
	} else {
		p.Printf("%s %d defined\n", p.green("✓"), len(cfg.Languages))
		for name, lang := range cfg.Languages {
			if len(lang.Detection.Files) == 0 && lang.Detection.Pattern == "" {
				result.AddWarning("Language '%s' has no detection rules", name)
			}
		}
	}

	// 3. Check targets
	p.Printf("  Checking targets... ")
	if len(cfg.Targets) == 0 {
		result.AddWarning("No targets defined")
		p.Println(p.yellow("none defined"))
	} else {
		p.Printf("%s %d defined\n", p.green("✓"), len(cfg.Targets))
	}
	targetNames := make(map[string]bool)
	for name := range cfg.Targets {
		targetNames[name] = true
	}

	// 4. Scan artifacts
	p.Printf("  Scanning artifacts... ")
	artifacts, err := internal.ScanArtifacts(rootPath, cfg)
	if err != nil {
		p.Println(p.red("✗"))
		result.AddError("Failed to scan artifacts: %v", err)
		return printCheckResult(p, result)
	}
	if len(artifacts) == 0 {
		p.Println(p.yellow("none found"))
		result.AddWarning("No artifacts found")
	} else {
		libs := 0
		for _, a := range artifacts {
			if a.Artifact.IsLib {
				libs++
			}
		}
		p.Printf("%s %d found (%d services, %d libraries)\n", p.green("✓"), len(artifacts), len(artifacts)-libs, libs)
	}

	// 5. Create artifact map
	artifactMap := make(map[string]internal.DiscoveredArtifact)
	for _, a := range artifacts {
		if existing, ok := artifactMap[a.Artifact.Name]; ok {
			result.AddError("Duplicate artifact name '%s' in:\n       - %s\n       - %s",
				a.Artifact.Name, existing.Path, a.Path)
		}
		artifactMap[a.Artifact.Name] = a
	}

	// 6. Check each artifact
	p.Printf("  Checking dependencies... ")
	depErrors := 0
	for _, a := range artifacts {
		// Check language detection
		if a.Language == "unknown" {
			result.AddWarning("Artifact '%s' has unknown language", a.Artifact.Name)
		}

		// Check target (only for non-libs)
		if !a.Artifact.IsLib {
			if a.Artifact.Target == "" {
				result.AddError("Artifact '%s' has no target defined", a.Artifact.Name)
			} else if !targetNames[a.Artifact.Target] {
				result.AddError("Artifact '%s' references unknown target '%s'",
					a.Artifact.Name, a.Artifact.Target)
			}
		}

		// Check dependencies
		for _, dep := range a.Artifact.Depends {
			if _, ok := artifactMap[dep]; !ok {
				result.AddError("Artifact '%s' depends on unknown artifact '%s'",
					a.Artifact.Name, dep)
				depErrors++
			}
		}
	}
	if depErrors == 0 {
		p.Printf("%s all resolved\n", p.green("✓"))
	} else {
		p.Printf("%s %d unresolved\n", p.red("✗"), depErrors)
	}

	// 7. Check for circular dependencies
	p.Printf("  Checking for cycles... ")
	cycles := findCycles(artifacts)
	if len(cycles) > 0 {
		p.Println(p.red("✗"))
		for _, cycle := range cycles {
			result.AddError("Circular dependency: %s", strings.Join(cycle, " → "))
		}
	} else {
		p.Printf("%s none\n", p.green("✓"))
	}

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
