// Package typegen only holds a test: it makes sure the TypeScript types that
// tygo generates from the Go data models (frontend/**/*.gen.ts) are up to date.
package typegen

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedTypesAreUpToDate regenerates all types into a temporary folder
// and compares them with the committed files.
func TestGeneratedTypesAreUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short mode (runs the go toolchain)")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found in PATH")
	}
	backend, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(filepath.Join(backend, "tygo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// tygo reads the Go sources in a subprocess, which Go's test cache cannot see.
	// Reading them here makes the cache notice every change, so a stale result is never reused.
	_ = filepath.WalkDir(filepath.Join(backend, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
			_, _ = os.ReadFile(path)
		}
		return nil
	})

	// Same config, but writing into a temp dir instead of ../frontend/.
	tmp := t.TempDir()
	redirected := strings.ReplaceAll(string(cfg), `"../frontend/`, `"`+filepath.ToSlash(tmp)+`/`)
	if redirected == string(cfg) {
		t.Fatal("tygo.yaml has no output_path under ../frontend/; the test cannot redirect it")
	}
	cfgPath := filepath.Join(tmp, "tygo.yaml")
	if err := os.WriteFile(cfgPath, []byte(redirected), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(goBin, "tool", "tygo", "--config", cfgPath, "generate")
	cmd.Dir = backend
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tygo failed: %v\n%s", err, out)
	}

	checked := 0
	err = filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".gen.ts") {
			return err
		}
		rel, _ := filepath.Rel(tmp, path)
		fresh, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		committed, err := os.ReadFile(filepath.Join(backend, "..", "frontend", rel))
		if err != nil {
			t.Errorf("frontend/%s is missing: run `go tool tygo generate` in backend/", filepath.ToSlash(rel))
			return nil
		}
		if normalize(fresh) != normalize(committed) {
			t.Errorf("frontend/%s is out of date: run `go tool tygo generate` in backend/", filepath.ToSlash(rel))
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Count(string(cfg), "output_path:"); checked != want {
		t.Errorf("generated %d files, tygo.yaml lists %d", checked, want)
	}
}

// normalize makes the comparison independent of line endings (git autocrlf).
func normalize(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }
