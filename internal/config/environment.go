package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// environmentNamePattern constrains a deployment environment name. The value
// becomes a bear.lock.yml key, the injected ENVIRONMENT variable and a
// positional CLI argument, so it stays short, lowercase and free of anything
// that would need quoting in a shell or in YAML.
var environmentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ValidateEnvironmentName checks the syntax of a deployment environment name
// without consulting a project configuration. Use it where the configuration is
// deliberately unavailable, such as apply replaying an approved plan.
func ValidateEnvironmentName(name string) error {
	if !environmentNamePattern.MatchString(name) {
		return fmt.Errorf("invalid environment name %q: use 1 to 32 characters matching [a-z][a-z0-9-]*", name)
	}
	return nil
}

// ValidateEnvironmentNames checks a list of environment names for syntax and
// rejects duplicates. It is the whole parse-time rule: a parser knows nothing
// about which environments a project declares.
func ValidateEnvironmentNames(environments []string) error {
	seen := make(map[string]bool, len(environments))
	for _, environment := range environments {
		if err := ValidateEnvironmentName(environment); err != nil {
			return err
		}
		if seen[environment] {
			return fmt.Errorf("duplicate environment %q", environment)
		}
		seen[environment] = true
	}
	return nil
}

// HasEnvironment reports whether the project declares this environment.
func (c *Config) HasEnvironment(name string) bool {
	return c != nil && slices.Contains(c.Environments, name)
}

// EnvironmentList renders the declared environments for an error message.
func (c *Config) EnvironmentList() string {
	if c == nil || len(c.Environments) == 0 {
		return "no environments"
	}
	return strings.Join(c.Environments, ", ")
}

// ValidateEnvironmentIn checks that a name is syntactically valid and declared
// by the project configuration. Use it wherever the configuration is available.
func ValidateEnvironmentIn(cfg *Config, name string) error {
	if err := ValidateEnvironmentName(name); err != nil {
		return err
	}
	if !cfg.HasEnvironment(name) {
		return fmt.Errorf("unknown environment %q: bear.config.yml declares %s", name, cfg.EnvironmentList())
	}
	return nil
}

// ValidateArtifactEnvironments checks that every entry of an artifact's
// deployment allowlist is declared by the project configuration. The parser
// cannot do this, so the graph loader does it for every command.
func ValidateArtifactEnvironments(cfg *Config, artifact string, environments []string) error {
	for _, environment := range environments {
		if !cfg.HasEnvironment(environment) {
			return fmt.Errorf("artifact %q allows undeclared environment %q; bear.config.yml declares %s", artifact, environment, cfg.EnvironmentList())
		}
	}
	return nil
}
