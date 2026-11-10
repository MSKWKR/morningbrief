// Package report renders the morning email (plain text + HTML).
//
// The email is a list of Sections so later features (e.g. a tech-news
// summary) can be added as one more section without touching the weather code.
package report

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
	"time"

	"morningbrief/internal/advice"
	"morningbrief/internal/weather"
)

type Section struct {
	Title string
	Text  string        // plain-text body
	HTML  template.HTML // HTML body (already safe)
}

type Email struct {
	Subject string
	Text    string
	HTML    string
}

// WeatherSection builds the weather + what-to-wear section.
func WeatherSection(today weather.DaySummary, history []weather.DaySummary, a advice.Advice) (Section, error) {
	var tb strings.Builder
	fmt.Fprintf(&tb, "%s  %s\n", today.Place, today.Date)
	fmt.Fprintf(&tb, "Temp %.0f–%.0f°C, feels like %.0f–%.0f°C, humidity ~%d%%, rain up to %d%%\n",
		today.TempMin, today.TempMax, today.FeelsMin, today.FeelsMax, today.HumidityAvg, today.RainChance)
	for _, p := range today.Periods {
		fmt.Fprintf(&tb, "  %s  %.0f–%.0f°C (feels %.0f–%.0f)  rain %d%%  %s\n",
			p.Name, p.TempMin, p.TempMax, p.FeelsMin, p.FeelsMax, p.RainChance, p.Condition)
	}
	tb.WriteString("\nWhat to wear\n")
	for _, s := range a.Wear {
		tb.WriteString("  - " + s + "\n")
	}
	if len(a.Bring) > 0 {
		tb.WriteString("Bring\n")
		for _, s := range a.Bring {
			tb.WriteString("  - " + s + "\n")
		}
	}
	tb.WriteString("Compared with earlier days\n")
	for _, s := range a.Comparison {
		tb.WriteString("  - " + s + "\n")
	}
	if len(history) > 0 {
		tb.WriteString("\nLast days (temp / feels-like / rain)\n")
		for _, h := range history[:min(len(history), 7)] {
			fmt.Fprintf(&tb, "  %s  %.0f–%.0f°  %.0f–%.0f°  %d%%\n",
				h.Date, h.TempMin, h.TempMax, h.FeelsMin, h.FeelsMax, h.RainChance)
		}
	}

	var hb bytes.Buffer
	err := tmpl.ExecuteTemplate(&hb, "weather.html", map[string]any{
		"T": today, "A": a, "H": history[:min(len(history), 7)],
	})
	if err != nil {
		return Section{}, err
	}
	return Section{Title: "Weather", Text: tb.String(), HTML: template.HTML(hb.String())}, nil
}

// Compose wraps sections into a full email.
func Compose(subject string, date time.Time, sections []Section) (Email, error) {
	var tb strings.Builder
	for _, s := range sections {
		tb.WriteString("== " + s.Title + " ==\n" + s.Text + "\n")
	}
	var hb bytes.Buffer
	if err := tmpl.ExecuteTemplate(&hb, "page.html", map[string]any{"Subject": subject, "Date": date.Format("01/02 Mon"), "Sections": sections}); err != nil {
		return Email{}, err
	}
	return Email{Subject: subject, Text: tb.String(), HTML: hb.String()}, nil
}

//go:embed template/*.html
var templateFS embed.FS

var funcs = template.FuncMap{
	"f0": func(v float64) string { return fmt.Sprintf("%.0f", v) },
}

var tmpl = template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "template/*.html"))
