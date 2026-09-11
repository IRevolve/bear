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

func Tree(configPath string, filterArtifacts []string) error {
	p := NewPrinter()

	cfg, err := internal.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	graph, err := internal.LoadGraph(rootPath, cfg)
	if err != nil {
		return fmt.Errorf("error loading artifacts: %w", err)
	}
	artifacts := graph.Artifacts

	// Load lock file for status info
	lockPath := filepath.Join(rootPath, "bear.lock.yml")
	lockFile, err := config.LoadLock(lockPath)
	if err != nil {
		return fmt.Errorf("error loading lock: %w", err)
	}

	// Build artifact map. LoadGraph already rejected duplicate names, so this
	// lookup can never collide.
	artifactMap := make(map[string]internal.DiscoveredArtifact, len(artifacts))
	for _, a := range artifacts {
		artifactMap[a.Artifact.Name] = a
	}
	for _, name := range filterArtifacts {
		if _, ok := artifactMap[name]; !ok {
			return fmt.Errorf("unknown artifact %q", name)
		}
	}

	// Build reverse dependency map (who depends on me?)
	dependents := make(map[string][]string)
	for _, a := range artifacts {
		for _, dep := range a.Artifact.Depends {
			dependents[dep] = append(dependents[dep], a.Artifact.Name)
		}
	}

	p.BearHeader(fmt.Sprintf("Dependency Tree: %s", cfg.Name))

	// Filter or show all
	if len(filterArtifacts) > 0 {
		// Show specific artifacts
		for i, name := range filterArtifacts {
			if a, ok := artifactMap[name]; ok {
				if i > 0 {
					p.Blank()
				}
				printArtifactTree(p, cfg.Environments, a, artifactMap, dependents, lockFile, "", true)
			} else {
				p.Warning(fmt.Sprintf("Unknown artifact: %s", name))
			}
		}
	} else {
		// Full tree: Show from libraries to services
		printFullDependencyTree(p, cfg.Environments, artifacts, artifactMap, dependents, lockFile)
	}

	// Statistics
	libs := 0
	for _, a := range artifacts {
		if a.Artifact.IsLib {
			libs++
		}
	}
	p.Blank()
	p.Println(p.dim(strings.Repeat("─", 40)))
	p.Printf("  Total: %d artifacts (%d services, %d libraries)\n",
		len(artifacts), len(artifacts)-libs, libs)

	return nil
}

// printFullDependencyTree displays the complete dependency tree
func printFullDependencyTree(p *Printer, environments []string, artifacts []internal.DiscoveredArtifact, artifactMap map[string]internal.DiscoveredArtifact, dependents map[string][]string, lockFile *config.LockFile) {
	// Group: libraries first, then services
	var libs, services []internal.DiscoveredArtifact
	for _, a := range artifacts {
		if a.Artifact.IsLib {
			libs = append(libs, a)
		} else {
			services = append(services, a)
		}
	}

	sort.Slice(libs, func(i, j int) bool { return libs[i].Artifact.Name < libs[j].Artifact.Name })
	sort.Slice(services, func(i, j int) bool { return services[i].Artifact.Name < services[j].Artifact.Name })

	// Libraries
	if len(libs) > 0 {
		p.Printf("  %s\n", p.dim("Libraries"))
		for _, a := range libs {
			deps := dependents[a.Artifact.Name]
			sort.Strings(deps)
			status := getStatus(p, environments, a, lockFile)
			p.Printf("   %s%s\n", p.bold(a.Artifact.Name), status)
			if len(deps) > 0 {
				p.Printf("      └─ used by: %s\n", p.dim(strings.Join(deps, ", ")))
			}
		}
		p.Blank()
	}

	// Services
	if len(services) > 0 {
		p.Printf("  %s\n", p.dim("Services"))
		for _, a := range services {
			status := getStatus(p, environments, a, lockFile)
			target := ""
			if a.Artifact.Target != "" {
				target = p.dim(fmt.Sprintf(" → %s", a.Artifact.Target))
			}
			p.Printf("   %s%s%s\n", p.bold(a.Artifact.Name), target, status)

			// Dependencies
			if len(a.Artifact.Depends) > 0 {
				deps := append([]string{}, a.Artifact.Depends...)
				sort.Strings(deps)
				for i, dep := range deps {
					connector := "├─"
					if i == len(deps)-1 {
						connector = "└─"
					}
					depLabel := dep
					if d, ok := artifactMap[dep]; ok && d.Artifact.IsLib {
						depLabel = p.dim(dep)
					}
					p.Printf("      %s %s\n", connector, depLabel)
				}
			}
		}
	}
}

func getStatus(p *Printer, environments []string, a internal.DiscoveredArtifact, lockFile *config.LockFile) string {
	if lockFile == nil {
		return ""
	}
	var statuses []string
	// Only the configured environments are shown. History for an environment
	// that is no longer declared stays in the lock file as historical data.
	for _, environment := range environments {
		if entry, ok := lockFile.Environments[environment][a.Artifact.Name]; ok {
			version := entry.Version
			if version == "" {
				version = entry.Commit
			}
			status := environment + ": " + version
			if entry.Pinned {
				status += " (pinned)"
			}
			statuses = append(statuses, status)
		}
	}
	if len(statuses) > 0 {
		return p.dim(" [" + strings.Join(statuses, "; ") + "]")
	}
	return ""
}

// printArtifactTree prints the tree for a specific artifact (dependencies)
func printArtifactTree(p *Printer, environments []string, a internal.DiscoveredArtifact, artifactMap map[string]internal.DiscoveredArtifact, dependents map[string][]string, lockFile *config.LockFile, prefix string, isRoot bool) {
	status := getStatus(p, environments, a, lockFile)
	extra := ""

	if !a.Artifact.IsLib && a.Artifact.Target != "" {
		extra = p.dim(fmt.Sprintf(" → %s", a.Artifact.Target))
	}

	if isRoot {
		p.Printf("  %s%s%s\n", p.bold(a.Artifact.Name), extra, status)
	} else {
		p.Printf("%s%s%s\n", a.Artifact.Name, extra, status)
	}

	// Print dependencies
	deps := append([]string{}, a.Artifact.Depends...)
	if len(deps) > 0 {
		sort.Strings(deps)
		for i, depName := range deps {
			isLast := i == len(deps)-1
			connector := "├── "
			childPrefix := prefix + "│   "
			if isLast {
				connector = "└── "
				childPrefix = prefix + "    "
			}

			if dep, ok := artifactMap[depName]; ok {
				p.Printf("%s%s", prefix, connector)
				printArtifactTree(p, environments, dep, artifactMap, dependents, lockFile, childPrefix, false)
			} else {
				p.Printf("%s%s%s (not found)\n", prefix, connector, p.red(depName))
			}
		}
	}

	// Print dependents if root
	if isRoot {
		deps := dependents[a.Artifact.Name]
		if len(deps) > 0 {
			p.Blank()
			p.Printf("   ⬆  Used by: %s\n", p.dim(strings.Join(deps, ", ")))
		}
	}
}
