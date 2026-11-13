package main

import (
	"context"
	"errors"
	"log"
	"time"

	"morningbrief/internal/digest"
	"morningbrief/internal/report"
)

// The feeds behind the themed sections. Each limit is how many items one
// source may contribute; with the 24h window below, quiet sources show fewer.
// All of them were checked against robots.txt and answer plain HTTP requests.
var (
	cyberSources = []digest.Source{
		{Name: "The Hacker News", URL: "https://feeds.feedburner.com/TheHackersNews", Max: 3},
		{Name: "Krebs on Security", URL: "https://krebsonsecurity.com/feed/", Max: 2},
	}
	marketSources = []digest.Source{
		{Name: "MarketWatch", URL: "https://feeds.content.dowjones.io/public/rss/mw_topstories", Max: 3},
		{Name: "CNBC Tech", URL: "https://www.cnbc.com/id/19854910/device/rss/rss.html", Max: 2},
		{Name: "STAT", URL: "https://www.statnews.com/feed/", Max: 2},
		{Name: "BioPharma Dive", URL: "https://www.biopharmadive.com/feeds/news/", Max: 2},
	}
)

const (
	// The job runs every 24 hours, so a 24h window means an item is never
	// shown on two days, with nothing stored to track it.
	feedWindow = 24 * time.Hour
	feedPause  = time.Second
)

// themed is one themed section plus, if it could not be produced at all, why.
type themed struct {
	sec     report.Section
	failure string // empty unless the whole section was unavailable
}

func feedOptions(now time.Time) digest.Options {
	return digest.Options{Now: now, MaxAge: feedWindow, Pause: feedPause, Logf: log.Printf}
}

// buildCyber builds the Cybersecurity section: CISA's newly exploited
// vulnerabilities plus security headlines. Either half may fail on its own.
func buildCyber(ctx context.Context, f digest.DocFetcher, now time.Time) themed {
	const title = "Cybersecurity"
	d := report.BriefData{
		Title:   title,
		Sources: []string{"CISA KEV", "The Hacker News", "Krebs on Security"},
	}

	kev, kevErr := digest.KEV(ctx, f, now)
	if kevErr != nil {
		log.Printf("cyber: KEV: %v", kevErr)
		kev = report.KEVBlock{Note: "CISA list unavailable today (" + reasonOf(kevErr) + ")"}
	}
	d.KEV = &kev

	heads, headErr := digest.Headlines(ctx, f, cyberSources, feedOptions(now))
	switch {
	case headErr != nil:
		log.Printf("cyber: headlines: %v", headErr)
		d.HeadlinesNote = "Headlines unavailable today (" + reasonOf(headErr) + ")"
	case len(heads) == 0:
		d.HeadlinesNote = "No new headlines in the last 24 hours"
	default:
		d.Headlines = heads
	}

	if kevErr != nil && headErr != nil {
		return unavailable(title, "CISA list and security feeds both failed")
	}
	return render(d)
}

// buildMarkets builds the Markets section from stock-market and biotech
// headlines (the news behind VOO, CIBR and XBI).
func buildMarkets(ctx context.Context, f digest.DocFetcher, now time.Time) themed {
	const title = "Markets"
	heads, err := digest.Headlines(ctx, f, marketSources, feedOptions(now))
	if err != nil {
		log.Printf("markets: headlines: %v", err)
		return unavailable(title, reasonOf(err))
	}
	d := report.BriefData{
		Title:     title,
		Sources:   []string{"MarketWatch", "CNBC", "STAT", "BioPharma Dive"},
		Headlines: heads,
	}
	if len(heads) == 0 {
		d.HeadlinesNote = "No new headlines in the last 24 hours"
	}
	return render(d)
}

func render(d report.BriefData) themed {
	sec, err := report.BriefSection(d)
	if err != nil {
		log.Printf("%s: render: %v", d.Title, err)
		return unavailable(d.Title, "could not render section")
	}
	return themed{sec: sec}
}

func unavailable(title, reason string) themed {
	return themed{sec: report.SectionUnavailable(title, reason), failure: title + ": " + reason}
}

// reasonOf extracts the short reason from a digest error.
func reasonOf(err error) string {
	var ue *digest.UnavailableError
	if errors.As(err, &ue) {
		return ue.Reason
	}
	return "internal error"
}
