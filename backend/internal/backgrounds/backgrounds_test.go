package backgrounds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedNames(t *testing.T) {
	want := []string{"aurora", "dawn", "dusk", "meadow", "mist", "night"}
	got := Names()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for _, n := range got {
		pic, err := Load(n)
		if err != nil || len(pic.Data) == 0 || pic.ContentType != "image/jpeg" {
			t.Errorf("Load(%q) = %v, %v", n, pic, err)
		}
	}
}

func TestValidate(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "bg.png")
	os.WriteFile(png, []byte("x"), 0o600)
	txt := filepath.Join(dir, "bg.txt")
	os.WriteFile(txt, []byte("x"), 0o600)

	cases := map[string]string{
		"":                          "",
		"dawn":                      "",
		png:                         "",
		"dawn.jpg":                  "unknown background", // names have no extension
		"nope":                      "unknown background",
		txt:                         "not a picture",
		filepath.Join(dir, "x.png"): "cannot read",
		dir + string(os.PathSeparator) + "sub.png": "cannot read",
	}
	for in, want := range cases {
		err := Validate(in)
		switch {
		case want == "" && err != nil:
			t.Errorf("Validate(%q): %v", in, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("Validate(%q) = %v, want %q", in, err, want)
		}
	}
}

func TestLoadFileAndVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mine.webp")
	os.WriteFile(p, []byte("one"), 0o600)
	v1 := Version(p)
	pic, err := Load(p)
	if err != nil || string(pic.Data) != "one" || pic.ContentType != "image/webp" || pic.Version != v1 {
		t.Fatalf("Load = %+v, %v", pic, err)
	}
	os.WriteFile(p, []byte("changed"), 0o600)
	if Version(p) == v1 {
		t.Error("version must change when the file changes")
	}
}
