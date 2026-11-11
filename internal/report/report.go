// Package report renders the morning email (plain text + HTML).
//
// The email is a list of Sections so later features (e.g. a tech-news
// summary) can be added as one more section without touching the weather code.
package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

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
	fmt.Fprintf(&tb, "%s\n", today.Date)
	fmt.Fprintf(&tb, "Temp %s°C, feels like %s°C, humidity %d%%, rain up to %d%%\n",
		weather.TempRange(today.TempMin, today.TempMax), weather.TempRange(today.FeelsMin, today.FeelsMax), today.HumidityAvg, today.RainChance)
	for _, p := range today.Periods {
		fmt.Fprintf(&tb, "  %s  %s°C (feels %s)  rain %d%%  %s\n",
			p.Name, weather.TempRange(p.TempMin, p.TempMax), weather.TempRange(p.FeelsMin, p.FeelsMax), p.RainChance, p.Condition)
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
			fmt.Fprintf(&tb, "  %s  %s°  %s°  %d%%\n",
				h.Date, weather.TempRange(h.TempMin, h.TempMax), weather.TempRange(h.FeelsMin, h.FeelsMax), h.RainChance)
		}
	}

	var hb bytes.Buffer
	err := weatherTmpl.Execute(&hb, map[string]any{
		"T": today, "A": a, "H": history[:min(len(history), 7)],
	})
	if err != nil {
		return Section{}, err
	}
	return Section{Title: "Weather", Text: tb.String(), HTML: template.HTML(hb.String())}, nil
}

// Compose wraps sections into a full email.
func Compose(subject string, sections []Section) (Email, error) {
	var tb strings.Builder
	for _, s := range sections {
		tb.WriteString("== " + s.Title + " ==\n" + s.Text + "\n")
	}
	var hb bytes.Buffer
	hasNews := false
	for _, s := range sections {
		if s.Title == newsTitle {
			hasNews = true
		}
	}
	if err := pageTmpl.Execute(&hb, map[string]any{"Subject": subject, "Sections": sections, "HasNews": hasNews}); err != nil {
		return Email{}, err
	}
	return Email{Subject: subject, Text: tb.String(), HTML: hb.String()}, nil
}

var funcs = template.FuncMap{
	"rng": weather.TempRange,
}

// Email clients ignore <style> blocks inconsistently, so styles are inline.
var weatherTmpl = template.Must(template.New("w").Funcs(funcs).Parse(`
<p style="margin:0 0 4px;color:#666">{{.T.Date}}</p>
<p style="margin:0 0 12px;font-size:28px;font-weight:600">{{rng .T.TempMin .T.TempMax}}°C
 <span style="font-size:15px;font-weight:400;color:#555">feels {{rng .T.FeelsMin .T.FeelsMax}}° · humidity {{.T.HumidityAvg}}% · rain {{.T.RainChance}}%</span></p>
<table style="border-collapse:collapse;font-size:14px;margin-bottom:12px">
{{range .T.Periods}}<tr>
 <td style="padding:3px 10px 3px 0">{{.Name}}</td>
 <td style="padding:3px 10px">{{rng .TempMin .TempMax}}°</td>
 <td style="padding:3px 10px;color:#666">feels {{rng .FeelsMin .FeelsMax}}°</td>
 <td style="padding:3px 10px">☂ {{.RainChance}}%</td>
 <td style="padding:3px 0 3px 10px">{{.Condition}}</td>
</tr>{{end}}
</table>
<h3 style="margin:12px 0 4px;font-size:15px">What to wear</h3>
<ul style="margin:0;padding-left:20px">{{range .A.Wear}}<li>{{.}}</li>{{end}}</ul>
{{if .A.Bring}}<h3 style="margin:12px 0 4px;font-size:15px">Bring</h3>
<ul style="margin:0;padding-left:20px">{{range .A.Bring}}<li>{{.}}</li>{{end}}</ul>{{end}}
<h3 style="margin:12px 0 4px;font-size:15px">Compared with earlier days</h3>
<ul style="margin:0;padding-left:20px">{{range .A.Comparison}}<li>{{.}}</li>{{end}}</ul>
{{if .H}}<h3 style="margin:12px 0 4px;font-size:15px">Last days</h3>
<table style="border-collapse:collapse;font-size:13px;color:#444">
<tr style="color:#888"><td style="padding:2px 10px 2px 0">Date</td><td style="padding:2px 10px">Temp</td><td style="padding:2px 10px">Feels</td><td style="padding:2px 10px">Rain</td></tr>
{{range .H}}<tr><td style="padding:2px 10px 2px 0">{{.Date}}</td><td style="padding:2px 10px">{{rng .TempMin .TempMax}}°</td><td style="padding:2px 10px">{{rng .FeelsMin .FeelsMax}}°</td><td style="padding:2px 10px">{{.RainChance}}%</td></tr>{{end}}
</table>{{end}}
`))

var pageTmpl = template.Must(template.New("p").Parse(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>{{.Subject}}</title></head>
<body style="font-family:-apple-system,'Segoe UI','Noto Sans TC',sans-serif;color:#222;max-width:640px;margin:0 auto;padding:16px">
{{range .Sections}}<h2 style="font-size:18px;border-bottom:1px solid #ddd;padding-bottom:4px">{{.Title}}</h2>
{{.HTML}}{{end}}
<p style="margin-top:24px;font-size:12px;color:#999">{{if .HasNews}}Sources: 中央氣象署 Open Data (F-D0047) · Hacker News{{else}}Source: 中央氣象署 Open Data (F-D0047){{end}}</p>
</body></html>`))
