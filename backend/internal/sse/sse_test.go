package sse

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func serve(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) string {
	srv := httptest.NewServer(http.HandlerFunc(h))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestParsesEventsCommentsAndMultilineData(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, ": heartbeat\n\nevent: alive\ndata: {\"a\":1}\n\ndata: line1\ndata: line2\n\nevent: x\n\n")
	})
	var mu sync.Mutex
	var got []Event
	err := Connect(context.Background(), NewClient(), url, nil, time.Second, nil, func(e Event) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	})
	if err == nil {
		t.Error("a stream that ends must return an error so callers reconnect")
	}
	if len(got) != 2 || got[0] != (Event{"alive", `{"a":1}`}) || got[1] != (Event{"message", "line1\nline2"}) {
		t.Errorf("events: %+v", got)
	}
}

func TestIdleStreamIsDetected(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done() // never sends anything
	})
	start := time.Now()
	err := Connect(context.Background(), NewClient(), url, nil, 150*time.Millisecond, nil, func(Event) {})
	if !errors.Is(err, ErrIdle) {
		t.Fatalf("err = %v, want ErrIdle", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("idle detection too slow")
	}
}

func TestHeartbeatKeepsStreamAlive(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		for i := 0; i < 8; i++ {
			fmt.Fprint(w, ": ping\n")
			w.(http.Flusher).Flush()
			time.Sleep(50 * time.Millisecond)
		}
	})
	err := Connect(context.Background(), NewClient(), url, nil, 200*time.Millisecond, nil, func(Event) {})
	if errors.Is(err, ErrIdle) {
		t.Fatal("comments are heartbeats and must reset the idle timer")
	}
}

func TestHTTPErrorAndCancel(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 503) })
	if err := Connect(context.Background(), NewClient(), url, nil, time.Second, nil, func(Event) {}); err == nil {
		t.Error("non-200 must be an error")
	}

	url = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()
	if err := Connect(ctx, NewClient(), url, nil, 5*time.Second, nil, func(Event) {}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
