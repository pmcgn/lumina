// Package typegen only holds a test: it makes sure the TypeScript types that
// tygo generates from the Go data models (frontend/**/*.gen.ts) can be generated.
// The generated files are not committed (see .gitignore); the container build and
// developers run `go tool tygo generate` themselves.
package typegen

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTypesGenerate regenerates all types into a temporary folder and checks
// that every package in tygo.yaml produced its file.
func TestTypesGenerate(t *testing.T) {
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
