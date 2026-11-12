// Package news orchestrates the tech-news step: list HN stories, fetch each
// article, sanitize it, and summarize it.
//
// Failure tiers: a single story failing leaves that entry title-only (tier 1);
// news as a whole being unavailable returns *UnavailableError so the caller can
// show a one-line note (tier 2). Weather is handled elsewhere.
//
// Trust boundary: every piece of fetched or HN-provided text goes through
// internal/sanitize before it reaches the summarizer.
package news

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"morningbrief/internal/fetch"
	"morningbrief/internal/hn"
	"morningbrief/internal/report"
	"morningbrief/internal/sanitize"
	"morningbrief/internal/summarize"
)

// Lister selects the stories to show.
type Lister interface {
	Select(ctx context.Context, opts hn.SelectOptions) ([]hn.Item, error)
}

// TextFetcher returns the readable text of a page.
type TextFetcher interface {
	Text(ctx context.Context, url string) (string, error)
}

// Summarizer turns sanitized article text into a short summary.
type Summarizer interface {
	Summarize(ctx context.Context, title, text string) (string, error)
}

const (
	// Selection: the best-scoring stories submitted in the last 24h. There is
	// deliberately no minimum score: a slow day should give lower-scoring
	// stories, not an empty section. See hn.Client.Select for why a 24h window
	// (the job's own period) also prevents repeats across days.
	scanStories = 60
	wantStories = 5
	maxStoryAge = 24 * time.Hour

	maxTextRunes = 12000
	maxTitle     = 300
	minWords     = 150
	minWordsText = 20
)

// UnavailableError means news as a whole is unavailable (tier 2). Reason is
// composed by our code: short, no response bodies, no keys.
type UnavailableError struct{ Reason string }

func (e *UnavailableError) Error() string { return "news unavailable: " + e.Reason }

// Pipeline wires the news dependencies together.
type Pipeline struct {
	Lister     Lister
	Fetcher    TextFetcher
	Summarizer Summarizer
	Pause      time.Duration // between network fetches
	Logf       func(string, ...any)
}

// Run produces the news items. It errors only when Select fails, nothing
// qualifies, or the summarizer reports a fatal error.
func (p Pipeline) Run(ctx context.Context) ([]report.NewsItem, error) {
	logf := p.Logf
	if logf == nil {
		logf = log.Printf
	}

	stories, err := p.Lister.Select(ctx, hn.SelectOptions{Scan: scanStories, Want: wantStories, MaxAge: maxStoryAge})
	if err != nil {
		logf("news: select: %v", err)
		return nil, &UnavailableError{Reason: "Hacker News unreachable"}
	}
	if len(stories) == 0 {
		return nil, &UnavailableError{Reason: "no qualifying stories"}
	}

	out := make([]report.NewsItem, 0, len(stories))
	fetched := false
	deadlineLogged := false
	for _, it := range stories {
		item := report.NewsItem{
			Title:         sanitize.OneLine(it.Title, maxTitle),
			URL:           it.URL,
			DiscussionURL: hn.DiscussionURL(it.ID),
			Points:        it.Score,
			Comments:      it.Descendants,
		}
		textPost := it.URL == ""
		if textPost {
			item.URL = item.DiscussionURL
		}
		out = append(out, item)
		idx := len(out) - 1

		if ctx.Err() != nil {
			if !deadlineLogged {
				logf("news: deadline reached, remaining stories are title-only")
				deadlineLogged = true
			}
			continue
		}

		fail := func(reason string) {
			logf("news: story %d %s: %s", it.ID, item.URL, reason)
		}

		var text string
		minW := minWords
		if textPost {
			text = sanitize.HTMLFragmentToText(it.Text)
			minW = minWordsText
		} else {
			if fetched && p.Pause > 0 {
				t := time.NewTimer(p.Pause)
				select {
				case <-ctx.Done():
					t.Stop()
				case <-t.C:
				}
				if ctx.Err() != nil {
					continue // handled (and logged) at the next iteration
				}
			}
			fetched = true
			text, err = p.Fetcher.Text(ctx, it.URL)
			if err != nil {
				fail("fetch: " + fetchReason(err))
				continue
			}
		}
		if ctx.Err() != nil {
			continue
		}

		cleaned := sanitize.Clean(text, maxTextRunes)
		if n := sanitize.WordCount(cleaned); n < minW {
			fail(fmt.Sprintf("too little text (%d words)", n))
			continue
		}

		sum, err := p.Summarizer.Summarize(ctx, item.Title, cleaned)
		if err != nil {
			if summarize.IsFatal(err) {
				logf("news: story %d %s: fatal summarizer error: %v", it.ID, item.URL, err)
				return nil, &UnavailableError{Reason: err.Error()}
			}
			fail("summarize: " + err.Error())
			continue
		}
		out[idx].Summary = sum
	}
	return out, nil
}

// fetchReason names the error kind and, for HTTP failures, the status code.
func fetchReason(err error) string {
	var se *fetch.StatusError
	switch {
	case errors.As(err, &se):
		return fmt.Sprintf("HTTP status %d", se.Code)
	case errors.Is(err, fetch.ErrBlockedAddress):
		return "blocked address"
	case errors.Is(err, fetch.ErrDisallowedByRobots):
		return "disallowed by robots.txt"
	case errors.Is(err, fetch.ErrTooLarge):
		return "response too large"
	case errors.Is(err, fetch.ErrNoContent):
		return "no readable content"
	case errors.Is(err, fetch.ErrContentType):
		return "unsupported content type"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	return "network error"
}
