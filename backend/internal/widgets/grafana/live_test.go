package grafana

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

// TestLive runs real panels of a Grafana instance through the widget. It only
// reads. Skipped unless GRAFANA_URL and GRAFANA_TOKEN are set (never commit the
// token). GRAFANA_DASHBOARD is a uid or title; GRAFANA_PANELS is a "|" separated
// list of panel ids or titles.
func TestLive(t *testing.T) {
	url, token := os.Getenv("GRAFANA_URL"), os.Getenv("GRAFANA_TOKEN")
	dash, panels := os.Getenv("GRAFANA_DASHBOARD"), os.Getenv("GRAFANA_PANELS")
	if url == "" || token == "" || dash == "" || panels == "" {
		t.Skip("GRAFANA_URL / GRAFANA_TOKEN / GRAFANA_DASHBOARD / GRAFANA_PANELS not set")
	}
	for _, ref := range strings.Split(panels, "|") {
		p := &Provider{client: &http.Client{Timeout: 30 * time.Second}, now: time.Now}
		if err := p.Start(context.Background(), widget.Env{
			Settings:   map[string]any{"dashboard": dash, "panel": ref},
			Connection: map[string]any{"url": url, "token": token},
		}); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		v, err := p.Fetch(context.Background())
		if err != nil {
			t.Errorf("panel %q: %v", ref, err)
			continue
		}
		d := v.(Data)
		t.Logf("panel %q -> %q [%s] unit=%q thresholds=%d min=%v max=%v, %d series in %v, warnings=%v",
			ref, d.Title, d.Display, d.Unit, len(d.Thresholds), ptr(d.Min), ptr(d.Max), len(d.Series), time.Since(start).Round(time.Millisecond), d.Warnings)
		for _, s := range d.Series {
			last := s.Points[len(s.Points)-1]
			t.Logf("    %-32q %4d points, last %.2f at %s", s.Name, len(s.Points), last.V, time.UnixMilli(last.T).Format("15:04"))
		}
	}
}

func ptr(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
