package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IRevolve/Bear/internal/config"
	"github.com/IRevolve/Bear/internal/loader"
	"github.com/IRevolve/Bear/internal/scanner"
)

func Tree(configPath string, filterArtifacts []string) error {
	cfg, err := loader.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	artifacts, err := scanner.ScanArtifacts(rootPath, cfg)
	if err != nil {
		return fmt.Errorf("error scanning artifacts: %w", err)
	}

	// Load lock file for status info
	lockPath := filepath.Join(rootPath, "bear.lock.yml")
	lockFile, _ := config.LoadLock(lockPath)

	// Build artifact map
	artifactMap := make(map[string]scanner.DiscoveredArtifact)
	for _, a := range artifacts {
		artifactMap[a.Artifact.Name] = a
	}

	// Build reverse dependency map (who depends on me?)
	dependents := make(map[string][]string)
	for _, a := range artifacts {
		for _, dep := range a.Artifact.DependsOn {
			dependents[dep] = append(dependents[dep], a.Artifact.Name)
		}
	}

	fmt.Println()
	fmt.Printf("🐻 Dependency Tree: %s\n", cfg.Name)
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	// Filter or show all
	if len(filterArtifacts) > 0 {
		// Show specific artifacts
		for i, name := range filterArtifacts {
			if a, ok := artifactMap[name]; ok {
				if i > 0 {
					fmt.Println()
				}
				printArtifactTree(a, artifactMap, dependents, lockFile, "", true)
			} else {
				fmt.Printf("⚠️  Unknown artifact: %s\n", name)
			}
		}
	} else {
		// Full tree: Show from libraries to services
		printFullDependencyTree(artifacts, artifactMap, dependents, lockFile)
	}

	// Statistics
	libs := 0
	for _, a := range artifacts {
		if a.Artifact.IsLib {
			libs++
		}
	}
	fmt.Println(strings.Repeat("─", 50))
	fmt.Printf("Total: %d artifacts (%d services, %d libraries)\n",
		len(artifacts), len(artifacts)-libs, libs)

	return nil
}

// printFullDependencyTree displays the complete dependency tree
func printFullDependencyTree(artifacts []scanner.DiscoveredArtifact, artifactMap map[string]scanner.DiscoveredArtifact, dependents map[string][]string, lockFile *config.LockFile) {
	// Gruppiere: Libraries zuerst, dann Services
	var libs, services []scanner.DiscoveredArtifact
	for _, a := range artifacts {
		if a.Artifact.IsLib {
			libs = append(libs, a)
		} else {
			services = append(services, a)
		}
	}

	// Sortiere
	sort.Slice(libs, func(i, j int) bool { return libs[i].Artifact.Name < libs[j].Artifact.Name })
	sort.Slice(services, func(i, j int) bool { return services[i].Artifact.Name < services[j].Artifact.Name })

	// Libraries
	if len(libs) > 0 {
		fmt.Println("📚 Libraries")
		for _, a := range libs {
			deps := dependents[a.Artifact.Name]
			sort.Strings(deps)
			status := getStatus(a, lockFile)
			fmt.Printf("   %s%s\n", a.Artifact.Name, status)
			if len(deps) > 0 {
				fmt.Printf("      └─ used by: %s\n", strings.Join(deps, ", "))
			}
		}
		fmt.Println()
	}

	// Services
	if len(services) > 0 {
		fmt.Println("📦 Services")
		for _, a := range services {
			status := getStatus(a, lockFile)
			target := ""
			if a.Artifact.Target != "" {
				target = fmt.Sprintf(" → %s", a.Artifact.Target)
			}
			fmt.Printf("   %s%s%s\n", a.Artifact.Name, target, status)

			// Dependencies
			if len(a.Artifact.DependsOn) > 0 {
				deps := a.Artifact.DependsOn
				sort.Strings(deps)
				for i, dep := range deps {
					connector := "├─"
					if i == len(deps)-1 {
						connector = "└─"
					}
					depIcon := "📦"
					if d, ok := artifactMap[dep]; ok && d.Artifact.IsLib {
						depIcon = "📚"
					}
					fmt.Printf("      %s %s %s\n", connector, depIcon, dep)
				}
			}
		}
	}
}

func getStatus(a scanner.DiscoveredArtifact, lockFile *config.LockFile) string {
	if lockFile == nil {
		return ""
	}
	if lockFile.IsPinned(a.Artifact.Name) {
		return " 📌"
	}
	if entry, ok := lockFile.Artifacts[a.Artifact.Name]; ok {
		return fmt.Sprintf(" [%s]", entry.Version)
	}
	return ""
}

// findRoots finds all artifacts that are base libraries (no own deps, but others depend on them)
func findRoots(artifacts []scanner.DiscoveredArtifact, dependents map[string][]string) []scanner.DiscoveredArtifact {
	var roots []scanner.DiscoveredArtifact

	for _, a := range artifacts {
		// Libraries without own dependencies that are used by others
		if len(a.Artifact.DependsOn) == 0 && len(dependents[a.Artifact.Name]) > 0 {
			roots = append(roots, a)
		}
	}

	// Sortiere alphabetisch
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Artifact.Name < roots[j].Artifact.Name
	})

	return roots
}

// printFullTree prints the complete tree with dependents (unused, kept for reference)
func printFullTree(a scanner.DiscoveredArtifact, artifactMap map[string]scanner.DiscoveredArtifact, dependents map[string][]string, lockFile *config.LockFile, prefix string, isLast bool, printed map[string]bool) {
	if printed[a.Artifact.Name] {
		return
	}
	printed[a.Artifact.Name] = true

	// Print current node
	connector := "├── "
	if isLast {
		connector = "└── "
	}
	if prefix == "" {
		connector = ""
	}

	icon := "📦"
	if a.Artifact.IsLib {
		icon = "📚"
	}

	status := ""
	if lockFile != nil {
		if lockFile.IsPinned(a.Artifact.Name) {
			status = " 📌"
		} else if entry, ok := lockFile.Artifacts[a.Artifact.Name]; ok {
			status = fmt.Sprintf(" [%s]", entry.Version)
		}
	}

	fmt.Printf("%s%s%s %s%s\n", prefix, connector, icon, a.Artifact.Name, status)

	// Berechne neuen Prefix
	newPrefix := prefix
	if prefix != "" {
		if isLast {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}
	}

	// Print dependents (who depends on me?)
	deps := dependents[a.Artifact.Name]
	sort.Strings(deps)

	for i, depName := range deps {
		if dep, ok := artifactMap[depName]; ok {
			isLastDep := i == len(deps)-1
			printFullTree(dep, artifactMap, dependents, lockFile, newPrefix, isLastDep, printed)
		}
	}
}

// printArtifactTree prints the tree for a specific artifact (dependencies)
func printArtifactTree(a scanner.DiscoveredArtifact, artifactMap map[string]scanner.DiscoveredArtifact, dependents map[string][]string, lockFile *config.LockFile, prefix string, isRoot bool) {
	icon := "📦"
	if a.Artifact.IsLib {
		icon = "📚"
	}

	status := getStatus(a, lockFile)
	extra := ""

	if !a.Artifact.IsLib && a.Artifact.Target != "" {
		extra = fmt.Sprintf(" → %s", a.Artifact.Target)
	}

	if isRoot {
		fmt.Printf("%s %s%s%s\n", icon, a.Artifact.Name, extra, status)
	} else {
		fmt.Printf("%s%s%s\n", a.Artifact.Name, extra, status)
	}

	// Print dependencies
	deps := a.Artifact.DependsOn
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
				depIcon := "📦"
				if dep.Artifact.IsLib {
					depIcon = "📚"
				}
				fmt.Printf("%s%s%s ", prefix, connector, depIcon)
				printArtifactTree(dep, artifactMap, dependents, lockFile, childPrefix, false)
			} else {
				fmt.Printf("%s%s❓ %s (not found)\n", prefix, connector, depName)
			}
		}
	}

	// Print dependents if root
	if isRoot {
		deps := dependents[a.Artifact.Name]
		if len(deps) > 0 {
			fmt.Println()
			fmt.Printf("   ⬆️  Used by: %s\n", strings.Join(deps, ", "))
		}
	}
}

func printArtifactInfo(a scanner.DiscoveredArtifact, lockFile *config.LockFile, prefix string) {
	icon := "📦"
	if a.Artifact.IsLib {
		icon = "📚"
	}

	status := ""
	if lockFile != nil {
		if lockFile.IsPinned(a.Artifact.Name) {
			status = " 📌"
		} else if entry, ok := lockFile.Artifacts[a.Artifact.Name]; ok {
			status = fmt.Sprintf(" [%s]", entry.Version)
		}
	}

	target := ""
	if !a.Artifact.IsLib && a.Artifact.Target != "" {
		target = fmt.Sprintf(" → %s", a.Artifact.Target)
	}

	fmt.Printf("%s%s %s%s%s\n", prefix, icon, a.Artifact.Name, target, status)
}
