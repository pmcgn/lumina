// Package history is a small in-memory time-series store (docs/requirements.md
// FR-25a). It is bounded by design: one point per series per minute, a
// retention window, and a cap on the number of series. Nothing is persisted.
package history

import (
	"sort"
	"sync"
	"time"
)

const (
	Resolution       = time.Minute
	DefaultRetention = 24 * time.Hour
	DefaultMaxSeries = 256
)

type Point struct {
	T int64   `json:"t"` // unix milliseconds
	V float64 `json:"v"`
}

type Store struct {
	retention time.Duration
	maxSeries int

	mu     sync.Mutex
	series map[string][]Point
}

// New creates a store. Zero values select the defaults.
func New(retention time.Duration, maxSeries int) *Store {
	if retention <= 0 {
		retention = DefaultRetention
	}
	if maxSeries <= 0 {
		maxSeries = DefaultMaxSeries
	}
	return &Store{retention: retention, maxSeries: maxSeries, series: map[string][]Point{}}
}

// Add records a value. Values within the same minute replace each other (the
// latest wins), older-than-last timestamps are ignored. It returns false if a
// new series would exceed the series cap.
func (s *Store) Add(name string, t time.Time, v float64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	pts, ok := s.series[name]
	if !ok && len(s.series) >= s.maxSeries {
		return false
	}
	bucket := t.Truncate(Resolution).UnixMilli()
	p := Point{T: bucket, V: v}
	switch n := len(pts); {
	case n > 0 && bucket == pts[n-1].T:
		pts[n-1] = p
	case n > 0 && bucket < pts[n-1].T:
		return true // out of order, drop
	default:
		pts = append(pts, p)
	}
	// Drop everything older than the retention window.
	cut := t.Add(-s.retention).UnixMilli()
	if i := sort.Search(len(pts), func(i int) bool { return pts[i].T >= cut }); i > 0 {
		pts = append(pts[:0:0], pts[i:]...) // copy so the old backing array can be freed
	}
	s.series[name] = pts
	return true
}

// Backfill replaces a series with points obtained from the source's own
// history (e.g. Grafana). Points need not be sorted.
func (s *Store) Backfill(name string, pts []Point) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.series[name]; !ok && len(s.series) >= s.maxSeries {
		return false
	}
	out := make([]Point, 0, len(pts))
	for _, p := range pts {
		out = append(out, Point{T: time.UnixMilli(p.T).Truncate(Resolution).UnixMilli(), V: p.V})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T < out[j].T })
	// Collapse duplicates created by truncation (latest wins).
	dedup := out[:0]
	for _, p := range out {
		if n := len(dedup); n > 0 && dedup[n-1].T == p.T {
			dedup[n-1] = p
		} else {
			dedup = append(dedup, p)
		}
	}
	s.series[name] = dedup
	return true
}

// Query returns the points in [from, to], reduced to at most maxPoints with
// the LTTB algorithm (keeps the visual shape of the curve). maxPoints <= 0
// disables downsampling.
func (s *Store) Query(name string, from, to time.Time, maxPoints int) []Point {
	s.mu.Lock()
	pts := s.series[name]
	lo := sort.Search(len(pts), func(i int) bool { return pts[i].T >= from.UnixMilli() })
	hi := sort.Search(len(pts), func(i int) bool { return pts[i].T > to.UnixMilli() })
	sel := append([]Point(nil), pts[lo:hi]...)
	s.mu.Unlock()
	return Downsample(sel, maxPoints)
}

// Len returns the number of stored series and points (for diagnostics).
func (s *Store) Len() (series, points int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.series {
		points += len(p)
	}
	return len(s.series), points
}

// Downsample reduces pts to n points using Largest-Triangle-Three-Buckets.
func Downsample(pts []Point, n int) []Point {
	if n <= 0 || len(pts) <= n || n < 3 {
		if n > 0 && n < 3 && len(pts) > n {
			return append([]Point(nil), pts[:n]...)
		}
		return pts
	}
	out := make([]Point, 0, n)
	out = append(out, pts[0])
	bucketSize := float64(len(pts)-2) / float64(n-2)
	a := 0
	for i := 0; i < n-2; i++ {
		start := int(float64(i)*bucketSize) + 1
		end := int(float64(i+1)*bucketSize) + 1
		// Average of the next bucket is the third triangle corner.
		nStart := end
		nEnd := int(float64(i+2)*bucketSize) + 1
		if nEnd > len(pts) {
			nEnd = len(pts)
		}
		var avgT, avgV float64
		for _, p := range pts[nStart:nEnd] {
			avgT += float64(p.T)
			avgV += p.V
		}
		cnt := float64(nEnd - nStart)
		avgT, avgV = avgT/cnt, avgV/cnt

		best, bestArea := start, -1.0
		for j := start; j < end; j++ {
			area := abs((float64(pts[a].T)-avgT)*(pts[j].V-pts[a].V) - (float64(pts[a].T)-float64(pts[j].T))*(avgV-pts[a].V))
			if area > bestArea {
				best, bestArea = j, area
			}
		}
		out = append(out, pts[best])
		a = best
	}
	return append(out, pts[len(pts)-1])
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
