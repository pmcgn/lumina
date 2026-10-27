package openhab

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/sse"
	"github.com/pmcgn/lumina/internal/widget"
)

// TestLiveReadOnly reads a real openHAB: it loads the sitemap and listens to the
// event stream for a few seconds. It NEVER sends commands. Skipped unless
// OPENHAB_URL and OPENHAB_SITEMAP are set.
func TestLiveReadOnly(t *testing.T) {
	url, sitemap := os.Getenv("OPENHAB_URL"), os.Getenv("OPENHAB_SITEMAP")
	if url == "" || sitemap == "" {
		t.Skip("OPENHAB_URL / OPENHAB_SITEMAP not set")
	}
	p := &Provider{client: newClient(), stream: sse.NewClient()}
	if err := p.Start(context.Background(), widget.Env{
		Connection: map[string]any{"url": url},
		Settings:   map[string]any{"sitemap": sitemap},
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	emits := 0
	err := p.Stream(ctx, func(v any) {
		emits++
		d := v.(Data)
		if emits == 1 {
			t.Logf("title=%q, %d rows", d.Title, len(d.Rows))
			for _, r := range d.Rows {
				t.Logf("  %-7s %-28q value=%-10q on=%-5v locked=%-5v icon=%s mappings=%d", r.Kind, r.Label, r.Value, r.On, r.Locked, r.Icon, len(r.Mappings))
			}
		}
	})
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Fatalf("stream ended early: %v", err)
	}
	t.Logf("stream stayed connected for 8s, %d emit(s) (the first one is the initial load)", emits)
	if emits < 1 {
		t.Fatal("no data")
	}
}

func newClient() *http.Client { return &http.Client{Timeout: 10 * time.Second} }
