package history

import (
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func TestAddReplacesWithinMinuteAndIgnoresOutOfOrder(t *testing.T) {
	s := New(0, 0)
	s.Add("a", t0, 1)
	s.Add("a", t0.Add(20*time.Second), 2) // same minute: replaces
	s.Add("a", t0.Add(time.Minute), 3)
	s.Add("a", t0.Add(-time.Hour), 99) // older than last: dropped

	got := s.Query("a", t0.Add(-24*time.Hour), t0.Add(24*time.Hour), 0)
	if len(got) != 2 || got[0].V != 2 || got[1].V != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestRetention(t *testing.T) {
	s := New(time.Hour, 0)
	for i := 0; i < 180; i++ {
		s.Add("a", t0.Add(time.Duration(i)*time.Minute), float64(i))
	}
	_, points := s.Len()
	if points > 61 {
		t.Fatalf("retention not applied, %d points", points)
	}
	got := s.Query("a", t0, t0.Add(24*time.Hour), 0)
	if got[0].V < 119 {
		t.Fatalf("oldest point should be ~1h old, got %v", got[0])
	}
}

func TestSeriesCap(t *testing.T) {
	s := New(0, 2)
	if !s.Add("a", t0, 1) || !s.Add("b", t0, 1) {
		t.Fatal("first two series must fit")
	}
	if s.Add("c", t0, 1) {
		t.Fatal("third series must be rejected")
	}
	if !s.Add("a", t0.Add(time.Minute), 2) {
		t.Fatal("existing series must keep working")
	}
}

func TestBackfillSortsAndDedups(t *testing.T) {
	s := New(0, 0)
	s.Backfill("a", []Point{
		{T: t0.Add(2 * time.Minute).UnixMilli(), V: 3},
		{T: t0.UnixMilli(), V: 1},
		{T: t0.Add(5 * time.Second).UnixMilli(), V: 2}, // same minute as t0
	})
	got := s.Query("a", t0.Add(-time.Hour), t0.Add(time.Hour), 0)
	if len(got) != 2 || got[0].V != 2 || got[1].V != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestDownsampleKeepsPeaksAndEnds(t *testing.T) {
	var pts []Point
	for i := 0; i < 1000; i++ {
		v := math.Sin(float64(i) / 50)
		if i == 500 {
			v = 10 // spike must survive
		}
		pts = append(pts, Point{T: int64(i) * 60_000, V: v})
	}
	out := Downsample(pts, 100)
	if len(out) != 100 {
		t.Fatalf("len = %d", len(out))
	}
	if out[0] != pts[0] || out[len(out)-1] != pts[len(pts)-1] {
		t.Error("first and last point must be kept")
	}
	found := false
	for _, p := range out {
		if p.V == 10 {
			found = true
		}
	}
	if !found {
		t.Error("spike lost by downsampling")
	}
	for i := 1; i < len(out); i++ {
		if out[i].T <= out[i-1].T {
			t.Fatal("output must stay ordered")
		}
	}
}

func TestDownsampleSmallInputUnchanged(t *testing.T) {
	pts := []Point{{T: 1, V: 1}, {T: 2, V: 2}}
	if got := Downsample(pts, 10); len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
}
