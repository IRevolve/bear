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

	fmt.Println()
	fmt.Println("🔍 Bear Configuration Check")
	fmt.Println("===========================")
	fmt.Println()

	// 1. Load config
	fmt.Print("📄 Loading config... ")
	cfg, err := internal.Load(configPath)
	if err != nil {
		fmt.Println("❌")
		result.AddError("Failed to load config: %v", err)
		return printResult(result)
	}
	fmt.Printf("✓ %s\n", cfg.Name)

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	// 2. Check languages
	fmt.Print("🔤 Checking languages... ")
	if len(cfg.Languages) == 0 {
		result.AddWarning("No languages defined")
		fmt.Println("⚠️  none defined")
	} else {
		fmt.Printf("✓ %d defined\n", len(cfg.Languages))
		for _, lang := range cfg.Languages {
			if len(lang.Detection.Files) == 0 && lang.Detection.Pattern == "" {
				result.AddWarning("Language '%s' has no detection rules", lang.Name)
			}
		}
	}

	// 3. Check targets
	fmt.Print("🎯 Checking targets... ")
	if len(cfg.Targets) == 0 {
		result.AddWarning("No targets defined")
		fmt.Println("⚠️  none defined")
	} else {
		fmt.Printf("✓ %d defined\n", len(cfg.Targets))
	}
	targetNames := make(map[string]bool)
	for _, t := range cfg.Targets {
		targetNames[t.Name] = true
	}

	// 4. Scan artifacts
	fmt.Print("📦 Scanning artifacts... ")
	artifacts, err := internal.ScanArtifacts(rootPath, cfg)
	if err != nil {
		fmt.Println("❌")
		result.AddError("Failed to scan artifacts: %v", err)
		return printResult(result)
	}
	if len(artifacts) == 0 {
		fmt.Println("⚠️  none found")
		result.AddWarning("No artifacts found")
	} else {
		libs := 0
		for _, a := range artifacts {
			if a.Artifact.IsLib {
				libs++
			}
		}
		fmt.Printf("✓ %d found (%d services, %d libraries)\n", len(artifacts), len(artifacts)-libs, libs)
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
	fmt.Print("🔗 Checking dependencies... ")
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
		for _, dep := range a.Artifact.DependsOn {
			if _, ok := artifactMap[dep]; !ok {
				result.AddError("Artifact '%s' depends on unknown artifact '%s'",
					a.Artifact.Name, dep)
				depErrors++
			}
		}
	}
	if depErrors == 0 {
		fmt.Println("✓ all resolved")
	} else {
		fmt.Printf("❌ %d unresolved\n", depErrors)
	}

	// 7. Check for circular dependencies
	fmt.Print("🔄 Checking for cycles... ")
	cycles := findCycles(artifacts)
	if len(cycles) > 0 {
		fmt.Println("❌")
		for _, cycle := range cycles {
			result.AddError("Circular dependency: %s", strings.Join(cycle, " → "))
		}
	} else {
		fmt.Println("✓ none")
	}

	fmt.Println()
	return printResult(result)
}

// findCycles finds circular dependencies
func findCycles(artifacts []internal.DiscoveredArtifact) [][]string {
	var cycles [][]string

	// Build adjacency map
	deps := make(map[string][]string)
	for _, a := range artifacts {
		deps[a.Artifact.Name] = a.Artifact.DependsOn
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

func printResult(result *ValidationResult) error {
	if len(result.Warnings) > 0 {
		fmt.Println("⚠️  Warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("   • %s\n", w)
		}
		fmt.Println()
	}

	if result.HasErrors() {
		fmt.Println("❌ Errors:")
		for _, e := range result.Errors {
			fmt.Printf("   • %s\n", e)
		}
		fmt.Println()
		fmt.Printf("Check failed with %d error(s)\n", len(result.Errors))
		return fmt.Errorf("validation failed")
	}

	fmt.Println("✅ All checks passed!")
	return nil
}
