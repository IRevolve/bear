package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListArgumentsSelectTree(t *testing.T) {
	oldDir, oldTree := workDir, showTree
	t.Cleanup(func() { workDir, showTree = oldDir, oldTree })
	workDir, showTree = t.TempDir(), false
	if err := os.WriteFile(filepath.Join(workDir, "bear.config.yml"), []byte("name: project\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := listCmd.RunE(listCmd, []string{"missing"}); err == nil || !strings.Contains(err.Error(), "unknown artifact") {
		t.Fatalf("artifact argument was ignored: %v", err)
	}
}
