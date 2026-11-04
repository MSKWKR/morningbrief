// Package store keeps one JSON file per day on disk:
//
//	<dir>/days/2026-11-04.json   the DaySummary (small, human-readable)
//	<dir>/raw/2026-11-04.json    the untouched CWA response (for re-parsing later)
//
// Files older than the retention window are deleted on each run.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"morningbrief/internal/weather"
)

const dateLayout = "2006-01-02"

type Store struct {
	Dir string
}

func New(dir string) (*Store, error) {
	for _, sub := range []string{"days", "raw"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{Dir: dir}, nil
}

// Save writes (or overwrites, if run twice in a day) today's files.
func (s *Store) Save(sum weather.DaySummary, raw []byte) error {
	b, err := json.MarshalIndent(sum, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(s.Dir, "days", sum.Date+".json"), b); err != nil {
		return err
	}
	if len(raw) > 0 {
		return writeAtomic(filepath.Join(s.Dir, "raw", sum.Date+".json"), raw)
	}
	return nil
}

// History returns up to n stored summaries strictly before `before`
// (a YYYY-MM-DD date), newest first.
func (s *Store) History(before string, n int) ([]weather.DaySummary, error) {
	entries, err := os.ReadDir(filepath.Join(s.Dir, "days"))
	if err != nil {
		return nil, err
	}
	var dates []string
	for _, e := range entries {
		d := strings.TrimSuffix(e.Name(), ".json")
		if _, err := time.Parse(dateLayout, d); err == nil && d < before {
			dates = append(dates, d) // YYYY-MM-DD sorts correctly as a string
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	if len(dates) > n {
		dates = dates[:n]
	}

	var out []weather.DaySummary
	for _, d := range dates {
		b, err := os.ReadFile(filepath.Join(s.Dir, "days", d+".json"))
		if err != nil {
			return nil, err
		}
		var sum weather.DaySummary
		if err := json.Unmarshal(b, &sum); err != nil {
			return nil, fmt.Errorf("%s: %w", d, err)
		}
		out = append(out, sum)
	}
	return out, nil
}

// Prune deletes files whose date is more than keepDays before today.
// With keepDays=30 and today=11-04, 10-05 is kept and 10-04 is deleted.
func (s *Store) Prune(today time.Time, keepDays int) (removed int, err error) {
	cutoff := today.AddDate(0, 0, -keepDays).Format(dateLayout)
	for _, sub := range []string{"days", "raw"} {
		dir := filepath.Join(s.Dir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return removed, err
		}
		for _, e := range entries {
			d := strings.TrimSuffix(e.Name(), ".json")
			if _, perr := time.Parse(dateLayout, d); perr != nil {
				continue // not one of ours; leave it alone
			}
			if d < cutoff {
				if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
					return removed, err
				}
				removed++
			}
		}
	}
	return removed, nil
}

// writeAtomic writes to a temp file then renames, so a crash mid-write
// never leaves a half-written JSON file behind.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
