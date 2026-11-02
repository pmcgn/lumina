package mqtt

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pmcgn/lumina/internal/widget"
)

// TestLive subscribes to a topic of a real broker and waits for a value. It
// only reads. Skipped unless MQTT_URL and MQTT_TOPIC are set; MQTT_USERNAME and
// MQTT_PASSWORD are optional (never commit them).
func TestLive(t *testing.T) {
	url, topic := os.Getenv("MQTT_URL"), os.Getenv("MQTT_TOPIC")
	if url == "" || topic == "" {
		t.Skip("MQTT_URL / MQTT_TOPIC not set")
	}
	p := &Provider{now: time.Now}
	err := p.Start(context.Background(), widget.Env{
		ID:       "live",
		Settings: map[string]any{"topic": topic, "valuePath": os.Getenv("MQTT_VALUE_PATH")},
		Connection: map[string]any{
			"url": url, "username": os.Getenv("MQTT_USERNAME"), "password": os.Getenv("MQTT_PASSWORD"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got := make(chan Data, 8)
	go p.Stream(ctx, func(v any) { got <- v.(Data) })
	for {
		select {
		case d := <-got:
			if len(d.Series) > 0 {
				pts := d.Series[0].Points
				t.Logf("%s = %v (warnings: %v)", topic, pts[len(pts)-1].V, d.Warnings)
				return
			}
			if len(d.Warnings) > 0 {
				t.Fatalf("message not readable: %v", d.Warnings)
			}
		case <-ctx.Done():
			t.Fatal("no message within 30s (a topic that is not retained may need to wait for the next publish)")
		}
	}
}
