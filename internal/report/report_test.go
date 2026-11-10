package report

import (
	"strings"
	"testing"

	"morningbrief/internal/advice"
	"morningbrief/internal/weather"
)

func TestTempRange(t *testing.T) {
	for _, c := range []struct {
		lo, hi float64
		want   string
	}{{19, 19, "19"}, {18.6, 19.4, "19"}, {18, 24, "18–24"}} {
		if got := weather.TempRange(c.lo, c.hi); got != c.want {
			t.Errorf("TempRange(%v, %v) = %q, want %q", c.lo, c.hi, got, c.want)
		}
	}
}

func TestWeatherSectionCleanup(t *testing.T) {
	day := weather.DaySummary{Date: "2026-11-10", Place: "臺北市大安區", TempMin: 19, TempMax: 19,
		FeelsMin: 19, FeelsMax: 20, HumidityAvg: 86, RainChance: 30}
	s, err := WeatherSection(day, nil, advice.Advice{Wear: []string{"x"}})
	if err != nil {
		t.Fatal(err)
	}
	html := string(s.HTML)
	for _, bad := range []string{"19–19", "~", "≤", "臺北市大安區"} {
		if strings.Contains(html, bad) {
			t.Errorf("HTML still contains %q", bad)
		}
	}
	for _, want := range []string{"19°C", "humidity 86%", "rain 30%"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}
}
