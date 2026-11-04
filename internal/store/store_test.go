package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"morningbrief/internal/weather"
)

func TestPruneKeeps30Days(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"2026-10-04", "2026-10-05", "2026-11-03"} {
		if err := s.Save(weather.DaySummary{Date: d}, nil); err != nil {
			t.Fatal(err)
		}
	}
	today := time.Date(2026, 11, 4, 7, 0, 0, 0, time.UTC)
	n, err := s.Prune(today, 30)
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, err %v; want 1", n, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "days", "2026-10-05.json")); err != nil {
		t.Errorf("10-05 should be kept: %v", err)
	}
	h, _ := s.History("2026-11-04", 30)
	if len(h) != 2 || h[0].Date != "2026-11-03" {
		t.Errorf("history = %+v", h)
	}
}
