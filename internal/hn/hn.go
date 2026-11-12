// Package hn is a small Hacker News API client plus story selection.
//
// HN data is only semi-trusted: titles, URLs and text are written by
// arbitrary submitters. Qualifies therefore validates URLs strictly, and
// callers must still sanitize everything before it reaches a model or an email.
package hn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://hacker-news.firebaseio.com/v0"
	// maxBody bounds every API response; real payloads are a few KiB.
	maxBody = 1 << 20
)

// ErrTooLarge is returned when a response exceeds the 1 MiB cap.
var ErrTooLarge = errors.New("hn: response too large")

// Item is a Hacker News item (story, job, comment, ...).
type Item struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Text        string `json:"text"` // HTML; sanitize before use
	Score       int    `json:"score"`
	Time        int64  `json:"time"` // unix seconds the story was submitted
	Descendants int    `json:"descendants"`
	Dead        bool   `json:"dead"`
	Deleted     bool   `json:"deleted"`
}

// Client talks to the HN Firebase API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
	// Logf is used to report skipped items; nil means log.Printf.
	Logf func(string, ...any)
}

// New returns a client for the public HN API.
func New() *Client {
	return &Client{
		BaseURL: defaultBaseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// get fetches path and returns the body, capped at maxBody. Errors never
// include the response body.
func (c *Client) get(ctx context.Context, p string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+p, nil)
	if err != nil {
		return nil, fmt.Errorf("hn: build request: %w", err)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		// Do's error embeds the URL, which is our own; fine to keep.
		return nil, fmt.Errorf("hn: request %s: %w", p, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("hn: %s: unexpected status %d", p, resp.StatusCode)
	}
	// Read one byte past the cap so "exactly at the limit" is still accepted.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("hn: read %s: %w", p, err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("%s: %w", p, ErrTooLarge)
	}
	return body, nil
}

// TopStoryIDs returns the current top story IDs, best first.
func (c *Client) TopStoryIDs(ctx context.Context) ([]int, error) {
	body, err := c.get(ctx, "/topstories.json")
	if err != nil {
		return nil, err
	}
	var ids []int
	if err := json.Unmarshal(body, &ids); err != nil {
		return nil, fmt.Errorf("hn: decode topstories: %w", err)
	}
	return ids, nil
}

// Item fetches one item. A null body (deleted/missing item) is an error.
func (c *Client) Item(ctx context.Context, id int) (Item, error) {
	p := "/item/" + strconv.Itoa(id) + ".json"
	body, err := c.get(ctx, p)
	if err != nil {
		return Item{}, err
	}
	var it *Item
	if err := json.Unmarshal(body, &it); err != nil {
		return Item{}, fmt.Errorf("hn: decode item %d: %w", id, err)
	}
	if it == nil {
		return Item{}, fmt.Errorf("hn: item %d: null response", id)
	}
	return *it, nil
}

var skippedExts = map[string]bool{
	".pdf": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".svg": true, ".mp4": true, ".mov": true, ".webm": true,
	".mp3": true, ".zip": true, ".gz": true,
}

// Video/social hosts whose pages yield no readable article text.
var skippedHosts = map[string]bool{
	"youtube.com": true, "youtu.be": true, "vimeo.com": true,
	"twitter.com": true, "x.com": true,
}

// Qualifies reports whether a story is worth summarizing.
func Qualifies(it Item) bool {
	if it.Type != "story" || it.Dead || it.Deleted || strings.TrimSpace(it.Title) == "" {
		return false
	}
	if it.URL == "" {
		return strings.TrimSpace(it.Text) != "" // text post
	}
	u, err := url.Parse(it.URL)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.User != nil || u.Hostname() == "" {
		return false
	}
	if skippedExts[strings.ToLower(path.Ext(u.Path))] {
		return false
	}
	// Exact match after stripping one www./m. prefix, so notyoutube.com
	// does not match youtube.com.
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")
	return !skippedHosts[host]
}

// DiscussionURL is the HN comments page for an item.
func DiscussionURL(id int) string {
	return "https://news.ycombinator.com/item?id=" + strconv.Itoa(id)
}

// SelectOptions controls which stories Select returns.
type SelectOptions struct {
	Scan     int           // how many top-story IDs to examine
	Want     int           // maximum number of stories to return
	MaxAge   time.Duration // submitted within this long before Now; 0 means no limit
	MinScore int           // minimum points
	Now      time.Time     // zero means time.Now()
}

// Select examines the first opts.Scan top stories sequentially and returns up
// to opts.Want of them, highest score first. A story is kept only if it
// Qualifies, was submitted within opts.MaxAge, has at least opts.MinScore
// points, and does not look like a resubmitted old article (LooksOld).
//
// Ranking by points among recent submissions (instead of taking front-page
// order) means a story that has sat on the front page for days drops out. With
// a daily job and MaxAge of 24h, a story that was included once is already too
// old the next day, so nothing repeats and no state has to be stored.
//
// Item fetch failures are logged and skipped; only a failing top-stories call
// is an error. On context cancellation it returns what it has so far.
func (c *Client) Select(ctx context.Context, opts SelectOptions) ([]Item, error) {
	ids, err := c.TopStoryIDs(ctx)
	if err != nil {
		return nil, err
	}
	if opts.Scan < len(ids) {
		ids = ids[:max(opts.Scan, 0)]
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	var out []Item
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		it, err := c.Item(ctx, id)
		if err != nil {
			c.logf("hn: skipping item %d: %v", id, err)
			continue
		}
		if !Qualifies(it) || it.Score < opts.MinScore || LooksOld(it, now) {
			continue
		}
		if opts.MaxAge > 0 && (it.Time == 0 || now.Sub(time.Unix(it.Time, 0)) > opts.MaxAge) {
			continue // too old, or no timestamp to prove it is fresh
		}
		out = append(out, it)
	}
	// Stable, so equal scores keep front-page order.
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if opts.Want >= 0 && len(out) > opts.Want {
		out = out[:opts.Want]
	}
	return out, nil
}

var (
	// HN convention puts the year of an old article in the title: "Foo (2013)".
	titleYearRe = regexp.MustCompile(`\(((?:19|20)\d{2})\)`)
	// Blog-style dated URLs: /2013/12/14/..., /posts/2013-12-14-title.
	urlDateRe = regexp.MustCompile(`(?:^|/)((?:19|20)\d{2})[/-](?:0[1-9]|1[0-2])(?:[/-]|$)`)
)

// LooksOld reports whether a story looks like an old article that was
// resubmitted: its title carries a "(YEAR)" tag, or its URL path carries a
// year/month date, from before last calendar year. Last year is allowed so a
// late-December article is still news in early January. Both signals are
// heuristics; a false positive just drops one story.
func LooksOld(it Item, now time.Time) bool {
	cutoff := now.Year() - 1
	for _, m := range titleYearRe.FindAllStringSubmatch(it.Title, -1) {
		if y, _ := strconv.Atoi(m[1]); y < cutoff {
			return true
		}
	}
	if u, err := url.Parse(it.URL); err == nil {
		if m := urlDateRe.FindStringSubmatch(u.Path); m != nil {
			if y, _ := strconv.Atoi(m[1]); y < cutoff {
				return true
			}
		}
	}
	return false
}
