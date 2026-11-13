// Package digest turns RSS/Atom feeds and CISA's Known Exploited
// Vulnerabilities (KEV) catalogue into the items shown in the email.
//
// Nothing here goes to a language model. Feed and catalogue text is still
// untrusted, because it ends up in an inbox, so every string passes through
// internal/sanitize, is length-capped, and links are validated as http(s).
// The KEV link is built from a validated CVE ID rather than taken from data.
package digest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"morningbrief/internal/feed"
	"morningbrief/internal/fetch"
	"morningbrief/internal/report"
	"morningbrief/internal/sanitize"
)

// DocFetcher is what *fetch.Fetcher provides.
type DocFetcher interface {
	Doc(ctx context.Context, url string, kind fetch.DocKind) ([]byte, error)
}

// Source is one feed and how many of its items to show.
type Source struct {
	Name string
	URL  string
	Max  int
}

// Options controls Headlines.
type Options struct {
	Now    time.Time     // zero means time.Now()
	MaxAge time.Duration // only items published within this long before Now
	Pause  time.Duration // between feeds
	Logf   func(string, ...any)
}

// UnavailableError means a whole block could not be produced. Reason is
// composed by our code: short, with no response bodies.
type UnavailableError struct{ Reason string }

func (e *UnavailableError) Error() string { return "unavailable: " + e.Reason }

const (
	maxTitleRunes   = 200
	maxSummaryRunes = 280
)

// boilerplate is the WordPress footer many feeds append to descriptions.
var boilerplate = regexp.MustCompile(`(?i)\s*The post .{0,300}? appeared first on .{0,200}?\.?\s*$`)

// Headlines fetches every source and returns recent items, source by source
// (each source newest first, at most its Max). A failing source is logged and
// skipped; it is an error only if every source failed. Items published more
// than MaxAge ago, undated items, and duplicate links are dropped. With a
// daily job and a 24h MaxAge an item cannot appear on two days.
func Headlines(ctx context.Context, f DocFetcher, srcs []Source, o Options) ([]report.Headline, error) {
	logf := o.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}

	var out []report.Headline
	seen := map[string]bool{}
	ok := 0
	for i, src := range srcs {
		if ctx.Err() != nil {
			break
		}
		if i > 0 && o.Pause > 0 {
			select {
			case <-ctx.Done():
			case <-time.After(o.Pause):
			}
		}
		data, err := f.Doc(ctx, src.URL, fetch.Feed)
		if err != nil {
			logf("digest: %s: %s", src.Name, reason(err))
			continue
		}
		entries, err := feed.Parse(data)
		if err != nil {
			logf("digest: %s: %v", src.Name, err)
			continue
		}
		ok++

		sort.SliceStable(entries, func(a, b int) bool { return entries[a].Published.After(entries[b].Published) })
		taken := 0
		for _, e := range entries {
			if taken >= src.Max {
				break
			}
			if e.Published.IsZero() || now.Sub(e.Published) > o.MaxAge || e.Published.After(now.Add(time.Hour)) {
				continue
			}
			link, valid := validLink(e.Link)
			title := sanitize.OneLine(e.Title, maxTitleRunes)
			if !valid || title == "" || seen[link] {
				continue
			}
			seen[link] = true
			taken++
			out = append(out, report.Headline{
				Title:   title,
				URL:     link,
				Source:  src.Name,
				Summary: summaryText(e.Summary),
			})
		}
	}
	if ok == 0 {
		return nil, &UnavailableError{Reason: fmt.Sprintf("none of %d feeds could be read", len(srcs))}
	}
	return out, nil
}

// summaryText turns a feed description (often HTML) into one short plain line.
func summaryText(raw string) string {
	t := sanitize.OneLine(sanitize.HTMLFragmentToText(raw), 2000)
	t = strings.TrimSpace(boilerplate.ReplaceAllString(t, ""))
	return truncate(t, maxSummaryRunes)
}

// truncate shortens a one-line string to at most max runes at a word boundary
// and marks the cut with an ellipsis.
func truncate(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	cut := sanitize.OneLine(s, max-1)
	return strings.TrimRight(cut, " ,;:.-") + "…"
}

// validLink accepts only http(s) URLs with a host and no userinfo.
func validLink(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2000 {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Hostname() == "" {
		return "", false
	}
	return u.String(), true
}

// reason turns a fetch error into a short log string with no body content.
func reason(err error) string {
	var se *fetch.StatusError
	switch {
	case errors.As(err, &se):
		return fmt.Sprintf("HTTP status %d", se.Code)
	case errors.Is(err, fetch.ErrDisallowedByRobots):
		return "disallowed by robots.txt"
	case errors.Is(err, fetch.ErrBlockedAddress):
		return "blocked address"
	case errors.Is(err, fetch.ErrContentType):
		return "unexpected content type"
	case errors.Is(err, fetch.ErrTooLarge):
		return "response too large"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}
	return "fetch failed"
}

// KEVURL is CISA's published catalogue.
const KEVURL = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"

const maxKEVEntries = 6

var cveRe = regexp.MustCompile(`^CVE-\d{4}-\d{4,7}$`)

type kevCatalog struct {
	Vulnerabilities []struct {
		CVEID             string `json:"cveID"`
		VendorProject     string `json:"vendorProject"`
		Product           string `json:"product"`
		VulnerabilityName string `json:"vulnerabilityName"`
		DateAdded         string `json:"dateAdded"`
		ShortDescription  string `json:"shortDescription"`
		Ransomware        string `json:"knownRansomwareCampaignUse"`
	} `json:"vulnerabilities"`
}

// KEV returns the vulnerabilities CISA added on the current UTC date. CISA
// publishes during the US day, and the job runs after that, so each entry is
// shown exactly once. If nothing was added, the block carries a note naming
// the most recent entry. Entries with known ransomware use come first.
func KEV(ctx context.Context, f DocFetcher, now time.Time) (report.KEVBlock, error) {
	data, err := f.Doc(ctx, KEVURL, fetch.JSON)
	if err != nil {
		return report.KEVBlock{}, &UnavailableError{Reason: reason(err)}
	}
	var cat kevCatalog
	if err := json.Unmarshal(data, &cat); err != nil || len(cat.Vulnerabilities) == 0 {
		return report.KEVBlock{}, &UnavailableError{Reason: "unreadable catalogue"}
	}
	if now.IsZero() {
		now = time.Now()
	}
	today := now.UTC().Format("2006-01-02")

	var entries []report.KEVEntry
	latestDate, latestCVE := "", ""
	for _, v := range cat.Vulnerabilities {
		if !cveRe.MatchString(v.CVEID) {
			continue
		}
		if _, err := time.Parse("2006-01-02", v.DateAdded); err != nil {
			continue
		}
		if v.DateAdded > latestDate {
			latestDate, latestCVE = v.DateAdded, v.CVEID
		}
		if v.DateAdded != today {
			continue
		}
		entries = append(entries, report.KEVEntry{
			CVE:         v.CVEID,
			URL:         "https://nvd.nist.gov/vuln/detail/" + v.CVEID,
			Vendor:      sanitize.OneLine(v.VendorProject, 80),
			Product:     sanitize.OneLine(v.Product, 80),
			Name:        sanitize.OneLine(v.VulnerabilityName, 160),
			Description: truncate(sanitize.OneLine(v.ShortDescription, 1000), maxSummaryRunes),
			DateAdded:   v.DateAdded,
			Ransomware:  v.Ransomware == "Known",
		})
	}
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].Ransomware != entries[b].Ransomware {
			return entries[a].Ransomware
		}
		return entries[a].CVE < entries[b].CVE
	})
	if len(entries) > maxKEVEntries {
		entries = entries[:maxKEVEntries]
	}
	block := report.KEVBlock{Entries: entries}
	if len(entries) == 0 {
		block.Note = "No new entries added today"
		if latestCVE != "" {
			block.Note += fmt.Sprintf(" (latest: %s, added %s)", latestCVE, latestDate)
		}
	}
	return block, nil
}
