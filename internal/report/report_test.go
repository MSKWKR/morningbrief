package report

import (
	"strings"
	"testing"
	"time"

	"morningbrief/internal/advice"
	"morningbrief/internal/weather"
)

func sampleDay() weather.DaySummary {
	return weather.DaySummary{
		Date: "2026-11-04", Place: "臺北市內湖區",
		TempMin: 18, TempMax: 24, FeelsMin: 17, FeelsMax: 25, RainChance: 60, HumidityAvg: 80,
		Periods: []weather.Period{{Name: "早上 06–12", TempMin: 18, TempMax: 21, FeelsMin: 17, FeelsMax: 21, RainChance: 60, Condition: "陰雨"}},
	}
}

func TestComposeHTMLAndText(t *testing.T) {
	w, err := WeatherSection(sampleDay(), []weather.DaySummary{sampleDay()}, advice.Advice{
		Wear: []string{"薄外套"}, Bring: []string{"雨傘"}, Comparison: []string{"比昨天涼"},
	})
	if err != nil {
		t.Fatal(err)
	}
	sections := []Section{w, {Title: "Tech", Text: "hello\n", HTML: "<p>tech body</p>"}}
	date := time.Date(2026, 11, 4, 7, 0, 0, 0, time.UTC)
	e, err := Compose("subj", date, sections)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Morning Brief · 11/04 Wed", "PingFang TC", "中央氣象署"} {
		if !strings.Contains(e.HTML, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
	for _, s := range sections {
		if !strings.Contains(e.HTML, s.Title) || !strings.Contains(e.HTML, string(s.HTML)) {
			t.Errorf("HTML missing section %q", s.Title)
		}
	}
	var want string
	for _, s := range sections {
		want += "== " + s.Title + " ==\n" + s.Text + "\n"
	}
	if e.Text != want {
		t.Errorf("Text = %q, want %q", e.Text, want)
	}
}

func TestWeatherSectionEscapes(t *testing.T) {
	d := sampleDay()
	d.Periods[0].Condition = "<script>alert(1)</script>"
	s, err := WeatherSection(d, nil, advice.Advice{Wear: []string{"<script>x</script>"}})
	if err != nil {
		t.Fatal(err)
	}
	h := string(s.HTML)
	if strings.Contains(h, "<script>") {
		t.Errorf("unescaped script in %s", h)
	}
	if !strings.Contains(h, "&lt;script&gt;") {
		t.Error("expected escaped script")
	}
	if strings.Contains(h, "Bring") || strings.Contains(h, "Last days") {
		t.Error("empty Bring/history sections should be omitted")
	}
}
