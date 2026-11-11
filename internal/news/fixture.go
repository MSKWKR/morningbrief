package news

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"

	"morningbrief/internal/report"
	"morningbrief/internal/sanitize"
	"morningbrief/internal/summarize"
)

type fixtureEntry struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	DiscussionURL string `json:"discussion_url"`
	Points        int    `json:"points"`
	Comments      int    `json:"comments"`
	Summary       string `json:"summary"`
}

// LoadFixture reads sample news items from a JSON file. No network, no API.
func LoadFixture(path string) ([]report.NewsItem, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []fixtureEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("news fixture %s: %w", path, err)
	}
	items := make([]report.NewsItem, 0, len(entries))
	for i, e := range entries {
		if err := checkURL(e.URL); err != nil {
			return nil, fmt.Errorf("news fixture entry %d: url: %w", i, err)
		}
		if err := checkURL(e.DiscussionURL); err != nil {
			return nil, fmt.Errorf("news fixture entry %d: discussion_url: %w", i, err)
		}
		it := report.NewsItem{
			Title:         sanitize.OneLine(e.Title, maxTitle),
			URL:           e.URL,
			DiscussionURL: e.DiscussionURL,
			Points:        e.Points,
			Comments:      e.Comments,
		}
		if e.Summary != "" {
			s, err := summarize.ValidateSummary(e.Summary)
			if err != nil {
				log.Printf("news fixture entry %d: summary dropped: %v", i, err)
			} else {
				it.Summary = s
			}
		}
		items = append(items, it)
	}
	return items, nil
}

func checkURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" || u.User != nil {
		return fmt.Errorf("must be http/https with a host and no userinfo")
	}
	return nil
}
