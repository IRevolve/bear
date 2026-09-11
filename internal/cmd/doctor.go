package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

// DoctorResult contains the result of a doctor run. Named distinctly from
// 'bear validate' (which validates build/test steps, not configuration) so
// the two are never confused.
type DoctorResult struct {
	Errors   []string
	Warnings []string
}

func (v *DoctorResult) AddError(format string, args ...interface{}) {
	v.Errors = append(v.Errors, fmt.Sprintf(format, args...))
}

func (v *DoctorResult) AddWarning(format string, args ...interface{}) {
	v.Warnings = append(v.Warnings, fmt.Sprintf(format, args...))
}

func (v *DoctorResult) HasErrors() bool {
	return len(v.Errors) > 0
}

func Doctor(configPath string) error {
	result := &DoctorResult{}
	p := NewPrinter()

	p.BearHeader("Doctor")

	checkSteps := []string{
		"Loading config",
		"Checking languages",
		"Checking targets",
		"Scanning artifacts",
		"Checking environments",
		"Checking dependencies",
		"Checking for cycles",
	}

	p.PhaseHeader("Diagnosing configuration")
	pt := NewProgressTracker(p, checkSteps)
	pt.SetOperation("doctor")
	pt.Start()

	// 1. Load config
	pt.MarkRunning(0)
	cfg, err := internal.Load(configPath)
	if err != nil {
		result.AddError("Failed to load config: %v", err)
		pt.MarkFailed(0, err, "")
		pt.Stop()
		return printDoctorResult(p, nil, result)
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
		return printDoctorResult(p, cfg, result)
	}
	if len(artifacts) == 0 {
		result.AddWarning("No artifacts found")
	}
	pt.MarkDone(3)

	// 5. Check environments. config.Load already enforced that the declaration
	// is nonempty, syntactically valid and duplicate-free, so all that is left
	// is the rule internal.LoadGraph enforces for plan and validate: every
	// allowlist entry must be declared. It is applied per artifact so one check
	// run lists every offender instead of only the first.
	pt.MarkRunning(4)
	environmentErrors := 0
	for _, a := range artifacts {
		if err := config.ValidateArtifactEnvironments(cfg, a.Artifact.Name, a.Artifact.Environments); err != nil {
			result.AddError("%s", err)
			environmentErrors++
		}
	}
	if environmentErrors > 0 {
		pt.MarkFailed(4, fmt.Errorf("%d undeclared environments", environmentErrors), "")
	} else {
		pt.MarkDone(4)
	}

	// 6. Create artifact map and check dependencies
	pt.MarkRunning(5)
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
		pt.MarkFailed(5, fmt.Errorf("%d unresolved dependencies", depErrors), "")
	} else {
		pt.MarkDone(5)
	}

	// 7. Check for circular dependencies
	pt.MarkRunning(6)
	cycles := findCycles(artifacts)
	if len(cycles) > 0 {
		for _, cycle := range cycles {
			result.AddError("Circular dependency: %s", strings.Join(cycle, " → "))
		}
		pt.MarkFailed(6, fmt.Errorf("circular dependencies found"), "")
	} else {
		pt.MarkDone(6)
	}

	pt.Stop()
	p.Blank()
	return printDoctorResult(p, cfg, result)
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

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		recStack[node] = true
		path = append(path, node)

		for _, dep := range deps[node] {
			if !visited[dep] {
				dfs(dep)
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
					cycle := append(append([]string{}, path[cycleStart:]...), dep)
					cycles = append(cycles, cycle)
				}
			}
		}

		path = path[:len(path)-1]
		recStack[node] = false
	}

	for _, a := range artifacts {
		if !visited[a.Artifact.Name] {
			path = []string{}
			dfs(a.Artifact.Name)
		}
	}

	return cycles
}

func printDoctorResult(p *Printer, cfg *config.Config, result *DoctorResult) error {
	// Deployment environments are project policy, so a check states them
	// explicitly rather than leaving the reader to open bear.config.yml.
	if cfg != nil {
		p.Printf("  %s %s\n", p.dim("Environments:"), cfg.EnvironmentList())
		p.Blank()
	}
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
		p.Printf("  %s\n", p.red(fmt.Sprintf("Doctor found %d error(s)", len(result.Errors))))
		return fmt.Errorf("doctor found %d error(s)", len(result.Errors))
	}

	p.Printf("  %s\n", p.green("All checks passed!"))
	return nil
}
