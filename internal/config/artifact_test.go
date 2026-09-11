package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadArtifactEnvironments(t *testing.T) {
	for _, tt := range []struct {
		name   string
		policy string
		want   []string
		bad    bool
	}{
		{name: "omitted"},
		{name: "empty", policy: "environments: []", want: []string{}},
		{name: "valid", policy: "environments: [dev, int, prd]", want: []string{"dev", "int", "prd"}},
		// The parser knows nothing about bear.config.yml, so any syntactically
		// valid name is accepted here; internal.LoadGraph rejects undeclared ones.
		{name: "undeclared name is a syntax-level pass", policy: "environments: [preprd]", want: []string{"preprd"}},
		{name: "digits and hyphens", policy: "environments: [eu-west-1, prd2]", want: []string{"eu-west-1", "prd2"}},
		{name: "duplicate", policy: "environments: [dev, dev]", bad: true},
		{name: "leading digit", policy: "environments: [2prd]", bad: true},
		{name: "leading hyphen", policy: "environments: [-prd]", bad: true},
		{name: "underscore", policy: "environments: [pre_prd]", bad: true},
		{name: "too long", policy: "environments: [" + strings.Repeat("a", 33) + "]", bad: true},
		{name: "case sensitive", policy: "environments: [PRD]", bad: true},
		{name: "empty name", policy: "environments: ['']", bad: true},
		{name: "whitespace", policy: "environments: [' int']", bad: true},
		{name: "scalar", policy: "environments: int", bad: true},
		{name: "number", policy: "environments: [123]", bad: true},
		{name: "map", policy: "environments: {int: true}", bad: true},
		{name: "old unknown key rejected", policy: "disabled_environments: []", bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bear.artifact.yml")
			if err := os.WriteFile(path, []byte("name: api\ntarget: local\n"+tt.policy+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			artifact, err := LoadArtifact(path)
			if tt.bad {
				if err == nil {
					t.Fatal("expected invalid policy to be rejected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(artifact.Environments, tt.want) {
				t.Errorf("environments = %v, want %v", artifact.Environments, tt.want)
			}
		})
	}
}

// The syntax check stands alone: it is the only rule apply can apply to a saved
// plan, and the only rule a parser can apply to an artifact file.
func TestValidateEnvironmentName(t *testing.T) {
	for _, environment := range []string{"dev", "int", "prd", "preprd", "eu-west-1", "a", strings.Repeat("a", 32)} {
		if err := ValidateEnvironmentName(environment); err != nil {
			t.Errorf("environment %q: %v", environment, err)
		}
	}
	for _, environment := range []string{"", " ", "production ", "INT", "dev ", "1dev", "-dev", "pre_prd", "pre.prd", strings.Repeat("a", 33)} {
		err := ValidateEnvironmentName(environment)
		if err == nil || !strings.Contains(err.Error(), "invalid environment name") {
			t.Errorf("environment %q: expected actionable error, got %v", environment, err)
		}
		if err != nil && !strings.Contains(err.Error(), `"`+environment+`"`) {
			t.Errorf("environment %q: error does not quote the offending value: %v", environment, err)
		}
	}
}

func TestValidateEnvironmentNames(t *testing.T) {
	if err := ValidateEnvironmentNames([]string{"dev", "preprd", "prd"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEnvironmentNames(nil); err != nil {
		t.Fatalf("an empty list is a syntax-level pass: %v", err)
	}
	err := ValidateEnvironmentNames([]string{"dev", "prd", "dev"})
	if err == nil || !strings.Contains(err.Error(), `duplicate environment "dev"`) {
		t.Fatalf("expected a duplicate error, got %v", err)
	}
}

// Membership is a separate concern from syntax and needs the project config.
func TestValidateEnvironmentIn(t *testing.T) {
	cfg := &Config{Environments: []string{"preprd", "prd"}}
	if !cfg.HasEnvironment("preprd") || cfg.HasEnvironment("dev") {
		t.Fatalf("HasEnvironment disagrees with %v", cfg.Environments)
	}
	if cfg.EnvironmentList() != "preprd, prd" {
		t.Fatalf("EnvironmentList = %q", cfg.EnvironmentList())
	}
	if (*Config)(nil).HasEnvironment("prd") {
		t.Fatal("a nil config declares nothing")
	}
	if (*Config)(nil).EnvironmentList() != "no environments" {
		t.Fatal("a nil config must still render a list")
	}
	for _, environment := range []string{"preprd", "prd"} {
		if err := ValidateEnvironmentIn(cfg, environment); err != nil {
			t.Errorf("environment %q: %v", environment, err)
		}
	}
	err := ValidateEnvironmentIn(cfg, "dev")
	if err == nil || err.Error() != `unknown environment "dev": bear.config.yml declares preprd, prd` {
		t.Fatalf("membership error = %v", err)
	}
	// A syntactically impossible name reports the syntax problem, not membership.
	if err := ValidateEnvironmentIn(cfg, "DEV"); err == nil || !strings.Contains(err.Error(), "invalid environment name") {
		t.Fatalf("syntax error = %v", err)
	}
}

func TestValidateArtifactEnvironments(t *testing.T) {
	cfg := &Config{Environments: []string{"dev", "int", "prd"}}
	if err := ValidateArtifactEnvironments(cfg, "api", []string{"dev", "prd"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateArtifactEnvironments(cfg, "api", nil); err != nil {
		t.Fatal(err)
	}
	err := ValidateArtifactEnvironments(cfg, "api", []string{"dev", "preprd"})
	if err == nil || err.Error() != `artifact "api" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd` {
		t.Fatalf("allowlist error = %v", err)
	}
}
