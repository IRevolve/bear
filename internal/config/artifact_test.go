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
		{name: "unknown", policy: "environments: [production]", bad: true},
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

func TestValidateEnvironment(t *testing.T) {
	for _, environment := range []string{"dev", "int", "prd"} {
		if err := ValidateEnvironment(environment); err != nil {
			t.Fatal(err)
		}
	}
	for _, environment := range []string{"", "production", "INT", "dev "} {
		if err := ValidateEnvironment(environment); err == nil || !strings.Contains(err.Error(), "expected dev, int, or prd") {
			t.Errorf("environment %q: expected actionable error, got %v", environment, err)
		}
	}
}
