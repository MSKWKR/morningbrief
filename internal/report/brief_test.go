package report

import (
	"strings"
	"testing"
)

func TestBriefSectionEscapesEverything(t *testing.T) {
	sec, err := BriefSection(BriefData{
		Title: "Cybersecurity",
		KEV: &KEVBlock{Entries: []KEVEntry{{
			CVE: "CVE-2026-1234", URL: "https://nvd.nist.gov/vuln/detail/CVE-2026-1234",
			Vendor: "<script>alert(1)</script>", Product: `"><img src=x onerror=alert(2)>`,
			Name: "<b>bold</b>", Description: "d & <i>e</i>", Ransomware: true,
		}}},
		Headlines: []Headline{
			{Title: "<script>alert(3)</script>", URL: "javascript:alert(4)", Source: "<u>src</u>", Summary: "<img onerror=alert(5)>"},
			{Title: "ok", URL: "https://example.com/a?x=1&y=2", Source: "S"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := string(sec.HTML)
	for _, bad := range []string{"<script", "<img", "<u>", "<i>", "<b>bold", "javascript:alert"} {
		if strings.Contains(h, bad) {
			t.Errorf("HTML contains unescaped %q:\n%s", bad, h)
		}
	}
	if !strings.Contains(h, "#ZgotmplZ") {
		t.Error("javascript: URL was not neutralized")
	}
	if !strings.Contains(h, "known ransomware use") {
		t.Error("ransomware flag missing")
	}
	if !strings.Contains(h, `href="https://example.com/a?x=1&amp;y=2"`) {
		t.Errorf("normal link missing/wrong:\n%s", h)
	}
}

func TestBriefSectionNotesAndBlocks(t *testing.T) {
	// Empty KEV shows its note; empty headlines show theirs.
	sec, err := BriefSection(BriefData{
		Title:         "Cybersecurity",
		KEV:           &KEVBlock{Note: "No new entries today (latest: CVE-2026-1 on 2026-11-10)"},
		HeadlinesNote: "No new headlines in the last 24 hours",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"No new entries today", "No new headlines in the last 24 hours"} {
		if !strings.Contains(sec.Text, want) || !strings.Contains(string(sec.HTML), want) {
			t.Errorf("missing %q in\ntext: %s\nhtml: %s", want, sec.Text, sec.HTML)
		}
	}
}

func TestSectionUnavailable(t *testing.T) {
	s := SectionUnavailable("Cybersecurity", "all feeds <failed>")
	if s.Title != "Cybersecurity" || !strings.Contains(s.Text, "Cybersecurity unavailable today (all feeds <failed>)") {
		t.Errorf("text = %q", s.Text)
	}
	if strings.Contains(string(s.HTML), "<failed>") || !strings.Contains(string(s.HTML), "&lt;failed&gt;") {
		t.Errorf("reason not escaped: %s", s.HTML)
	}
}

func TestFooterListsDistinctSources(t *testing.T) {
	a, _ := BriefSection(BriefData{Title: "Cybersecurity", Sources: []string{"CISA", "Krebs on Security"}, HeadlinesNote: "x"})
	b, _ := BriefSection(BriefData{Title: "Markets", Sources: []string{"MarketWatch", "Krebs on Security"}, HeadlinesNote: "x"})
	e, err := Compose("subj", []Section{{Title: "W", Text: "w", HTML: "w"}, a, b})
	if err != nil {
		t.Fatal(err)
	}
	want := "Sources: 中央氣象署 Open Data (F-D0047) · CISA · Krebs on Security · MarketWatch"
	if !strings.Contains(e.HTML, want) {
		t.Errorf("footer wrong:\n%s", e.HTML)
	}
	if strings.Count(e.HTML, "Krebs on Security") != 1 {
		t.Error("duplicate source in footer")
	}

	// With no themed sections the footer is unchanged from the weather-only email.
	e, _ = Compose("subj", []Section{{Title: "W", Text: "w", HTML: "w"}})
	if !strings.Contains(e.HTML, "Source: 中央氣象署 Open Data (F-D0047)") || strings.Contains(e.HTML, "Sources:") {
		t.Errorf("weather-only footer changed:\n%s", e.HTML)
	}
}
