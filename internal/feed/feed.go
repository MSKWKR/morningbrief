// Package feed parses RSS 2.0 and Atom documents into plain entries.
//
// Feed content is written by third parties and is untrusted: every field is
// returned as the raw string found in the document (descriptions may contain
// HTML). Callers must sanitize before display and validate links.
package feed

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// Entry is one feed item.
type Entry struct {
	Title     string
	Link      string
	Summary   string    // raw; may contain HTML
	Published time.Time // zero if missing or unparsable
}

// ErrNoEntries is returned when the document parsed but held no items.
var ErrNoEntries = errors.New("feed: no entries")

type document struct {
	Items   []rssItem   `xml:"channel>item"`
	Entries []atomEntry `xml:"entry"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	DCDate      string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// Parse decodes an RSS 2.0 or Atom document. The Go XML decoder does not
// expand external entities, and the caller bounds the document size.
func Parse(data []byte) ([]Entry, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charset.NewReaderLabel
	var d document
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("feed: parse: %w", err)
	}

	var out []Entry
	for _, it := range d.Items {
		out = append(out, Entry{
			Title:     strings.TrimSpace(it.Title),
			Link:      strings.TrimSpace(it.Link),
			Summary:   strings.TrimSpace(it.Description),
			Published: parseTime(first(it.PubDate, it.DCDate)),
		})
	}
	for _, e := range d.Entries {
		out = append(out, Entry{
			Title:     strings.TrimSpace(e.Title),
			Link:      atomHref(e.Links),
			Summary:   strings.TrimSpace(first(e.Summary, e.Content)),
			Published: parseTime(first(e.Published, e.Updated)),
		})
	}
	if len(out) == 0 {
		return nil, ErrNoEntries
	}
	return out, nil
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// atomHref prefers the alternate link (or one with no rel, which defaults to
// alternate) over enclosures, edit links and the like.
func atomHref(links []atomLink) string {
	for _, l := range links {
		if l.Rel == "" || l.Rel == "alternate" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

var timeLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822,
	"Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04 -0700", "2 Jan 2006 15:04:05 -0700",
	time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02",
}

func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
