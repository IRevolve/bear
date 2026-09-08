package config

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.yml")
	for _, data := range [][]byte{[]byte("private plan"), bytes.Repeat([]byte("replacement\n"), 1000), nil} {
		if err := WriteFileAtomic(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("read = %q, %v", got, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v, %v", entries, err)
	}
}

func TestWriteFileAtomicFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing-directory")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(path, "keep")
	if err := os.WriteFile(child, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("replacement"), 0600); err == nil {
		t.Fatal("expected rename failure")
	}
	got, err := os.ReadFile(child)
	if err != nil || string(got) != "original" {
		t.Fatalf("old destination modified: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files leaked: %v, %v", entries, err)
	}
	if err := WriteFileAtomic(filepath.Join(dir, "missing", "plan"), nil, 0600); err == nil {
		t.Fatal("expected missing parent error")
	}
}

func TestWriteFileAtomicReadersSeeCompleteFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows rename does not guarantee atomic replacement")
	}
	path := filepath.Join(t.TempDir(), "plan")
	a, b := bytes.Repeat([]byte("a"), 65536), bytes.Repeat([]byte("b"), 32768)
	if err := WriteFileAtomic(path, a, 0600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			got, err := os.ReadFile(path)
			if err != nil || (!bytes.Equal(got, a) && !bytes.Equal(got, b)) {
				t.Errorf("reader observed partial replacement: length %d, error %v", len(got), err)
				return
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()
	for i := 0; i < 30; i++ {
		data := a
		if i%2 == 0 {
			data = b
		}
		if err := WriteFileAtomic(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
