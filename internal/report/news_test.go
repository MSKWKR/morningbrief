package report

import (
	"strings"
	"testing"
)

func TestNewsSectionEscaping(t *testing.T) {
	s, err := NewsSection([]NewsItem{{
		Title:         "<script>alert(1)</script>",
		URL:           "https://example.com/a",
		DiscussionURL: "https://news.ycombinator.com/item?id=1",
		Summary:       "see <img src=x onerror=alert(1)>",
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := string(s.HTML)
	for _, bad := range []string{"<script", "<img"} {
		if strings.Contains(h, bad) {
			t.Errorf("HTML contains raw %q:\n%s", bad, h)
		}
	}
	for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "&lt;img src=x onerror=alert(1)&gt;"} {
		if !strings.Contains(h, want) {
			t.Errorf("HTML missing escaped %q:\n%s", want, h)
		}
	}
}

func TestNewsSectionBadScheme(t *testing.T) {
	s, err := NewsSection([]NewsItem{{Title: "t", URL: "javascript:alert(1)", DiscussionURL: "https://news.ycombinator.com/item?id=1"}})
	if err != nil {
		t.Fatal(err)
	}
	h := string(s.HTML)
	if strings.Contains(h, `href="javascript:`) {
		t.Errorf("executable javascript: href present:\n%s", h)
	}
	if !strings.Contains(h, "#ZgotmplZ") {
		t.Errorf("expected #ZgotmplZ replacement:\n%s", h)
	}
}

func TestNewsSectionEmptySummary(t *testing.T) {
	it := NewsItem{Title: "T", URL: "https://e.com", DiscussionURL: "https://hn/1", Points: 5, Comments: 2}
	s, err := NewsSection([]NewsItem{it})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(s.HTML), "<p "); got != 1 {
		t.Errorf("want only the meta paragraph, got %d <p> in:\n%s", got, s.HTML)
	}
	it.Summary = "Some summary."
	s, _ = NewsSection([]NewsItem{it})
	if got := strings.Count(string(s.HTML), "<p "); got != 2 {
		t.Errorf("want 2 paragraphs with summary, got %d", got)
	}
	if !strings.Contains(string(s.HTML), "5 points · 2 comments") {
		t.Errorf("meta line missing:\n%s", s.HTML)
	}
}

func TestNewsSectionText(t *testing.T) {
	s, err := NewsSection([]NewsItem{
		{Title: "One", URL: "https://a.com", DiscussionURL: "https://hn/1", Points: 10, Comments: 3, Summary: "Sum one."},
		{Title: "Two", URL: "https://b.com", DiscussionURL: "https://hn/2", Points: 1, Comments: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "1. One\n   https://a.com\n   Sum one.\n   10 points · 3 comments · https://hn/1\n" +
		"\n2. Two\n   https://b.com\n   1 points · 0 comments · https://hn/2\n"
	if s.Text != want {
		t.Errorf("text mismatch:\n got %q\nwant %q", s.Text, want)
	}
	if s.Title != "Tech news" {
		t.Errorf("title = %q", s.Title)
	}
}

func TestNewsUnavailableSection(t *testing.T) {
	s := NewsUnavailableSection("API <b>error</b> 401")
	if s.Title != "Tech news" {
		t.Errorf("title = %q", s.Title)
	}
	if !strings.Contains(s.Text, "Tech news unavailable today (API <b>error</b> 401)") {
		t.Errorf("text = %q", s.Text)
	}
	h := string(s.HTML)
	if strings.Contains(h, "<b>") || !strings.Contains(h, "Tech news unavailable today (API &lt;b&gt;error&lt;/b&gt; 401)") {
		t.Errorf("reason not escaped: %s", h)
	}
}

func TestComposeFooter(t *testing.T) {
	weather := Section{Title: "Weather", Text: "w", HTML: "<p>w</p>"}
	e, err := Compose("s", []Section{weather})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.HTML, "Source: 中央氣象署 Open Data (F-D0047)<") || strings.Contains(e.HTML, "Hacker News") {
		t.Errorf("footer without news wrong:\n%s", e.HTML)
	}
	news, _ := NewsSection([]NewsItem{{Title: "t", URL: "https://a.com"}})
	for _, n := range []Section{news, NewsUnavailableSection("x")} {
		e, err = Compose("s", []Section{weather, n})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(e.HTML, "Sources: 中央氣象署 Open Data (F-D0047) · Hacker News") {
			t.Errorf("footer with news wrong:\n%s", e.HTML)
		}
	}
}
