// Package backgrounds resolves the configured background pictures: either the
// name of an embedded picture (file name without extension, e.g. "dawn") or a
// file path (e.g. a volume mounted into the container).
package backgrounds

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed images
var images embed.FS

// Extensions of the picture formats browsers can show as a CSS background.
var extensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".avif": true, ".gif": true, ".svg": true}

// Picture is a resolved background, ready to be served.
type Picture struct {
	Data        []byte
	ContentType string
	ModTime     time.Time
	Version     string // changes when the picture changes (cache busting)
}

// IsPath reports whether a configured value is a file path instead of the name of an embedded picture.
func IsPath(v string) bool { return strings.ContainsAny(v, `/\`) || filepath.VolumeName(v) != "" }

// Names lists the embedded pictures (file names without extension).
func Names() []string {
	entries, _ := fs.ReadDir(images, "images")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
	}
	sort.Strings(names)
	return names
}

// Validate checks a configured value without loading the picture. An empty value means "no picture".
func Validate(v string) error {
	if v == "" {
		return nil
	}
	if IsPath(v) {
		if !extensions[strings.ToLower(filepath.Ext(v))] {
			return fmt.Errorf("%q is not a picture file (jpg, png, webp, avif, gif, svg)", v)
		}
		st, err := os.Stat(v)
		if err != nil {
			return fmt.Errorf("cannot read %q: %v", v, err)
		}
		if st.IsDir() {
			return fmt.Errorf("%q is a directory", v)
		}
		return nil
	}
	if _, err := embedded(v); err != nil {
		return err
	}
	return nil
}

// Load reads the picture a configured value points to.
func Load(v string) (*Picture, error) {
	if IsPath(v) {
		return loadFile(v)
	}
	file, err := embedded(v)
	if err != nil {
		return nil, err
	}
	data, err := images.ReadFile(path.Join("images", file))
	if err != nil {
		return nil, err
	}
	// The embedded files change only with a new build, so the name identifies the version.
	return &Picture{Data: data, ContentType: contentType(file), Version: "e-" + v}, nil
}

// Version identifies the current state of the picture a value points to, without reading it.
// It is part of the URL sent to the browser so a replaced file is fetched again.
func Version(v string) string {
	if v == "" {
		return ""
	}
	if !IsPath(v) {
		return "e-" + v
	}
	st, err := os.Stat(v)
	if err != nil {
		return "missing"
	}
	return strconv.FormatInt(st.ModTime().UnixNano(), 36) + "-" + strconv.FormatInt(st.Size(), 36)
}

func loadFile(p string) (*Picture, error) {
	if !extensions[strings.ToLower(filepath.Ext(p))] {
		return nil, fmt.Errorf("%q is not a picture file", p)
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%q is a directory", p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return &Picture{Data: data, ContentType: contentType(p), ModTime: st.ModTime(), Version: Version(p)}, nil
}

// Reader returns the picture bytes as a seekable reader (for http.ServeContent).
func (p *Picture) Reader() *bytes.Reader { return bytes.NewReader(p.Data) }

// embedded finds the embedded file for a name; the name has no extension.
func embedded(name string) (string, error) {
	entries, _ := fs.ReadDir(images, "images")
	for _, e := range entries {
		if strings.TrimSuffix(e.Name(), path.Ext(e.Name())) == name {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("unknown background %q (embedded: %s; use a path for your own picture)", name, strings.Join(Names(), ", "))
}

func contentType(file string) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(file))); t != "" {
		return t
	}
	return "application/octet-stream"
}
