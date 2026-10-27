package vikunja

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

// TestLive runs against a real Vikunja instance. It is skipped unless
// VIKUNJA_URL and VIKUNJA_TOKEN are set (never commit the token). It creates
// one task, completes it, restores it, and deletes it again.
func TestLive(t *testing.T) {
	url, token := os.Getenv("VIKUNJA_URL"), os.Getenv("VIKUNJA_TOKEN")
	if url == "" || token == "" {
		t.Skip("VIKUNJA_URL / VIKUNJA_TOKEN not set")
	}
	ctx := context.Background()
	p := &Provider{client: (&Provider{}).client, now: time.Now}
	p.client = newTestClient()
	if err := p.Start(ctx, widget.Env{
		Settings:   map[string]any{"projects": []any{os.Getenv("VIKUNJA_PROJECT")}},
		Connection: map[string]any{"url": url, "token": token},
		Location:   time.Local,
	}); err != nil {
		t.Fatal(err)
	}

	title := "lumina live test " + time.Now().Format("150405")
	res, err := p.Action(ctx, "add", json.RawMessage(`{"title":"`+title+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	id := res.(map[string]int64)["id"]
	t.Logf("created task %d", id)
	cleanup := func() {
		_ = p.do(ctx, http.MethodDelete, "/tasks/"+itoa(id), nil, nil)
	}
	defer cleanup()

	find := func() (Task, bool) {
		d := fetch(t, p)
		for _, tk := range d.Tasks {
			if tk.ID == id {
				return tk, true
			}
		}
		return Task{}, false
	}
	tk, ok := find()
	if !ok || !strings.HasPrefix(tk.Title, "lumina live test") {
		t.Fatalf("new task not listed (ok=%v)", ok)
	}
	if _, err := p.Action(ctx, "complete", json.RawMessage(`{"id":`+itoa(id)+`}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := find(); ok {
		t.Error("completed task still listed")
	}
	if _, err := p.Action(ctx, "uncomplete", json.RawMessage(`{"id":`+itoa(id)+`}`)); err != nil {
		t.Fatal(err)
	}
	if _, ok := find(); !ok {
		t.Error("task not listed again after uncomplete")
	}
}

func itoa(i int64) string { return strconv.FormatInt(i, 10) }

func newTestClient() *http.Client { return &http.Client{Timeout: 15 * time.Second} }
