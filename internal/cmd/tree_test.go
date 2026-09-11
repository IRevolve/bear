package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func TestTreeRejectsCycles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bear.config.yml")
	writeEnvironmentFixture(t, path, "name: project\nenvironments: [dev]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "a", "bear.lib.yml"), "name: a\ndepends: [b]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "b", "bear.lib.yml"), "name: b\ndepends: [a]\n")
	for _, filter := range [][]string{nil, {"a"}} {
		if err := Tree(path, filter); err == nil || !strings.Contains(err.Error(), `dependency cycle involving "a"`) {
			t.Fatalf("expected cycle error, got %v", err)
		}
	}
}

// list and tree share LoadGraph with plan/validate, so a structurally broken
// project fails the same way for every command: a clear error and no partial
// output, rather than a misleading display of some (or all) artifacts.
func TestListAndTreeRejectInvalidGraphs(t *testing.T) {
	for _, tt := range []struct {
		name    string
		files   map[string]string
		wantErr string
	}{
		{
			name: "duplicate artifact name",
			files: map[string]string{
				"bear.config.yml": "name: project\nenvironments: [dev]\n",
				"a/bear.lib.yml":  "name: dup\n",
				"b/bear.lib.yml":  "name: dup\n",
			},
			wantErr: `duplicate artifact "dup"`,
		},
		{
			name: "unknown target",
			files: map[string]string{
				"bear.config.yml":       "name: project\nenvironments: [dev]\nlanguages: {test: {detection: {files: [bear.artifact.yml]}, steps: []}}\n",
				"api/bear.artifact.yml": "name: api\ntarget: missing\n",
			},
			wantErr: `artifact "api" references unknown target "missing"`,
		},
		{
			name: "undeclared environment in allowlist",
			files: map[string]string{
				"bear.config.yml":       "name: project\nenvironments: [dev]\nlanguages: {test: {detection: {files: [bear.artifact.yml]}, steps: []}}\ntargets: {local: {steps: [{name: deploy, run: 'true'}]}}\n",
				"api/bear.artifact.yml": "name: api\ntarget: local\nenvironments: [staging]\n",
			},
			wantErr: `artifact "api" allows undeclared environment "staging"; bear.config.yml declares dev`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, content := range tt.files {
				writeEnvironmentFixture(t, filepath.Join(root, rel), content)
			}
			path := filepath.Join(root, "bear.config.yml")
			for _, run := range []struct {
				name string
				fn   func() error
			}{
				{name: "list", fn: func() error { return List(path) }},
				{name: "tree", fn: func() error { return Tree(path, nil) }},
			} {
				t.Run(run.name, func(t *testing.T) {
					output, err := captureEnvironmentOutput(t, run.fn)
					if err == nil {
						t.Fatalf("expected error, got none; output: %s", output)
					}
					if !strings.Contains(err.Error(), tt.wantErr) {
						t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
					}
					if strings.TrimSpace(output) != "" {
						t.Fatalf("expected no output before the error, got: %s", output)
					}
				})
			}
		})
	}
}

func TestListAndTreeSurfaceLockErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bear.config.yml")
	writeEnvironmentFixture(t, path, "name: project\nenvironments: [dev]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "bear.lock.yml"), "environments: [broken")
	for _, run := range []func() error{func() error { return Tree(path, nil) }, func() error { return List(path) }} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "loading lock") {
			t.Fatalf("lock error was hidden: %v", err)
		}
	}
}

// The status line follows bear.config.yml, in declaration order. History for an
// environment the project no longer declares is retained but not shown.
func TestTreeEnvironmentStatus(t *testing.T) {
	p := NewPrinterWithWriter(&bytes.Buffer{})
	a := internal.DiscoveredArtifact{Artifact: &config.Artifact{Name: "api"}}
	lock := &config.LockFile{Environments: map[string]map[string]config.LockEntry{
		"prd":    {"api": {Version: "v1", Pinned: true}},
		"preprd": {"api": {Version: "v2"}},
		"dev":    {"api": {Version: "retired"}},
		"int":    {"other": {Version: "v3"}},
	}}
	for _, tt := range []struct {
		name         string
		environments []string
		want         string
	}{
		{name: "declared set", environments: []string{"preprd", "prd"}, want: " [preprd: v2; prd: v1 (pinned)]"},
		{name: "declaration order", environments: []string{"prd", "preprd"}, want: " [prd: v1 (pinned); preprd: v2]"},
		{name: "undeclared history is hidden", environments: []string{"prd"}, want: " [prd: v1 (pinned)]"},
		{name: "no history for the declared set", environments: []string{"int"}, want: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if status := getStatus(p, tt.environments, a, lock); status != tt.want {
				t.Fatalf("status = %q, want %q", status, tt.want)
			}
		})
	}
}

func TestFindCyclesRestoresTraversalState(t *testing.T) {
	var artifacts []internal.DiscoveredArtifact
	for _, a := range []config.Artifact{
		{Name: "a", Depends: []string{"b", "c"}},
		{Name: "b", Depends: []string{"a"}},
		{Name: "c", Depends: []string{"d"}},
		{Name: "d", Depends: []string{"c"}},
		{Name: "e", Depends: []string{"b"}},
	} {
		artifacts = append(artifacts, internal.DiscoveredArtifact{Artifact: &a})
	}
	cycles := findCycles(artifacts)
	if len(cycles) != 2 || strings.Join(cycles[0], ",") != "a,b,a" || strings.Join(cycles[1], ",") != "c,d,c" {
		t.Fatalf("cycles = %v", cycles)
	}
}

func TestDoctorRejectsEmptyDeployment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bear.config.yml")
	writeEnvironmentFixture(t, path, "name: project\nenvironments: [dev]\ntargets: {local: {steps: []}}\n")
	if err := Doctor(path); err == nil {
		t.Fatal("doctor accepted empty deployment target")
	}
}

// environmentReportFixture writes a project with a custom declared set, two
// artifacts and deployment history that includes a retired environment.
func environmentReportFixture(t *testing.T, declared, allowlist string) string {
	t.Helper()
	root := t.TempDir()
	writeEnvironmentFixture(t, filepath.Join(root, "bear.config.yml"), "name: project\nenvironments: ["+declared+"]\nlanguages: {test: {detection: {files: [bear.artifact.yml]}, steps: []}}\ntargets: {local: {steps: [{name: deploy, run: 'true'}]}}\n")
	writeEnvironmentFixture(t, filepath.Join(root, "api", "bear.artifact.yml"), "name: api\ntarget: local\ndepends: [shared]\nenvironments: ["+allowlist+"]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "libs", "shared", "bear.lib.yml"), "name: shared\n")
	lock := &config.LockFile{Environments: map[string]map[string]config.LockEntry{
		"preprd": {"api": {Commit: "aaaaaaa", Version: "v2", Target: "local"}},
		"prd":    {"api": {Commit: "bbbbbbb", Version: "v1", Target: "local", Pinned: true}},
		// History for an environment the project no longer declares is kept in
		// the lock file but never reported as current status.
		"int": {"api": {Commit: "ccccccc", Version: "retired", Target: "local"}},
	}}
	if err := lock.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "bear.config.yml")
}

// doctor states the declared environments and rejects an allowlist entry that
// names an environment the project does not declare.
func TestDoctorReportsEnvironments(t *testing.T) {
	for _, tt := range []struct {
		name      string
		declared  string
		allowlist string
		wantError bool
	}{
		{name: "declared", declared: "preprd, prd", allowlist: "preprd, prd"},
		{name: "undeclared", declared: "preprd, prd", allowlist: "preprd, dev", wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := environmentReportFixture(t, tt.declared, tt.allowlist)
			output, err := captureEnvironmentOutput(t, func() error { return Doctor(path) })
			if !strings.Contains(output, "Environments: "+tt.declared) {
				t.Errorf("doctor did not report the declared environments: %s", output)
			}
			if !tt.wantError {
				if err != nil {
					t.Fatalf("%v\n%s", err, output)
				}
				// Retired lock history is data, not a doctor failure.
				if strings.Contains(output, "int") {
					t.Errorf("doctor complained about retired lock history: %s", output)
				}
				return
			}
			if err == nil {
				t.Fatalf("doctor accepted an undeclared allowlist entry: %s", output)
			}
			want := `artifact "api" allows undeclared environment "dev"; bear.config.yml declares preprd, prd`
			if !strings.Contains(output, want) {
				t.Errorf("doctor output missing %q: %s", want, output)
			}
		})
	}
}

// list and tree report deployment status for the declared environments only.
func TestListAndTreeReportDeclaredEnvironments(t *testing.T) {
	path := environmentReportFixture(t, "preprd, prd", "preprd, prd")
	for _, run := range []struct {
		name string
		fn   func() error
	}{
		{name: "list", fn: func() error { return List(path) }},
		{name: "tree", fn: func() error { return Tree(path, nil) }},
		{name: "tree filtered", fn: func() error { return Tree(path, []string{"api"}) }},
	} {
		t.Run(run.name, func(t *testing.T) {
			output, err := captureEnvironmentOutput(t, run.fn)
			if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			if !strings.Contains(output, "[preprd: v2; prd: v1 (pinned)]") {
				t.Errorf("declared status missing: %s", output)
			}
			if strings.Contains(output, "retired") {
				t.Errorf("undeclared history was reported: %s", output)
			}
		})
	}
}
