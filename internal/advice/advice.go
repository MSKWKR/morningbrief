// Package advice turns today's summary plus recent history into
// "what to wear" suggestions. Rules use feels-like (體感) temperature,
// since Taiwan's humidity makes the raw number misleading.
package advice

import (
	"fmt"
	"math"
	"time"

	"morningbrief/internal/weather"
)

func dayBefore(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, -1).Format("2006-01-02")
}

type Advice struct {
	Headline   string   // one line for the email subject / top
	Wear       []string // clothing suggestions
	Bring      []string // umbrella, etc.
	Comparison []string // vs yesterday / vs last week
}

// Build expects history newest-first (as returned by store.History).
func Build(today weather.DaySummary, history []weather.DaySummary) Advice {
	var a Advice

	// Base layer from the warmest part of the day, outer layer from the coolest.
	a.Wear = append(a.Wear, baseLayer(today.FeelsMax))
	if outer := outerLayer(today.FeelsMin, today.RainChance); outer != "" {
		a.Wear = append(a.Wear, outer)
	}
	if swing := today.FeelsMax - today.FeelsMin; swing >= 7 {
		a.Wear = append(a.Wear, fmt.Sprintf("Big swing today (%.0f°): dress in layers you can take off.", swing))
	}
	if today.HumidityAvg >= 85 && today.FeelsMax >= 26 {
		a.Wear = append(a.Wear, "Muggy: pick breathable / quick-dry fabric.")
	}

	switch {
	case today.RainChance >= 60:
		a.Bring = append(a.Bring, fmt.Sprintf("Umbrella — %d%% chance of rain%s.", today.RainChance, rainyWhen(today)))
		a.Bring = append(a.Bring, "Shoes that can handle puddles.")
	case today.RainChance >= 30:
		a.Bring = append(a.Bring, fmt.Sprintf("Compact umbrella — %d%% chance of rain%s.", today.RainChance, rainyWhen(today)))
	}
	if today.WindMax >= 5 {
		a.Bring = append(a.Bring, fmt.Sprintf("Windy (Beaufort %d): windbreaker; a cheap umbrella may not survive.", today.WindMax))
	}

	a.Comparison = compare(today, history)
	a.Headline = headline(today, history)
	return a
}

func baseLayer(feelsMax float64) string {
	switch {
	case feelsMax >= 31:
		return "Hot: short sleeves, light breathable clothes."
	case feelsMax >= 27:
		return "Warm: T-shirt is enough."
	case feelsMax >= 23:
		return "Mild: T-shirt or thin long sleeves."
	case feelsMax >= 19:
		return "Cool: long sleeves."
	case feelsMax >= 15:
		return "Chilly: sweater or hoodie."
	case feelsMax >= 11:
		return "Cold: warm sweater plus a proper jacket."
	default:
		return "Very cold: heavy coat, consider scarf and gloves."
	}
}

func outerLayer(feelsMin float64, rainChance int) string {
	scarf := ""
	if rainChance < 30 {
		scarf = " and a scarf"
	}
	switch {
	case feelsMin < 13:
		return fmt.Sprintf("Morning/night feels like %.0f°: heavy jacket%s for the commute.", feelsMin, scarf)
	case feelsMin < 18:
		return fmt.Sprintf("Morning/night feels like %.0f°: wind resistant jacket%s.", feelsMin, scarf)
	case feelsMin < 22:
		return fmt.Sprintf("Morning/night feels like %.0f°: suit jacket.", feelsMin)
	case feelsMin < 24:
		return fmt.Sprintf("Morning/night feels like %.0f°: shirt.", feelsMin)
	}
	return ""
}

func rainyWhen(s weather.DaySummary) string {
	var when string
	for _, p := range s.Periods {
		if p.RainChance >= 30 {
			if when != "" {
				when += ", "
			}
			when += p.Name
		}
	}
	if when == "" {
		return ""
	}
	return " (" + when + ")"
}

func compare(today weather.DaySummary, history []weather.DaySummary) []string {
	if len(history) == 0 {
		return []string{"No earlier days stored yet — comparisons start tomorrow."}
	}
	var out []string
	y := history[0]
	label := "yesterday"
	if y.Date != dayBefore(today.Date) {
		label = "last stored day (" + y.Date + ")"
	}

	dMax := today.FeelsMax - y.FeelsMax
	dMin := today.FeelsMin - y.FeelsMin
	switch {
	case dMax <= -3 || dMin <= -3:
		out = append(out, fmt.Sprintf("Colder than %s (feels-like high %+.0f°, low %+.0f°) — add a layer vs what you wore.", label, dMax, dMin))
	case dMax >= 3 || dMin >= 3:
		out = append(out, fmt.Sprintf("Warmer than %s (feels-like high %+.0f°, low %+.0f°) — go a layer lighter.", label, dMax, dMin))
	default:
		out = append(out, fmt.Sprintf("About the same as %s (feels-like high %+.0f°, low %+.0f°) — same outfit works.", label, dMax, dMin))
	}
	if today.RainChance >= 50 && y.RainChance < 30 {
		out = append(out, fmt.Sprintf("Wetter than %s.", label))
	} else if today.RainChance < 30 && y.RainChance >= 50 {
		out = append(out, fmt.Sprintf("Drier than %s.", label))
	}

	if len(history) >= 3 {
		var sum float64
		n := min(len(history), 7)
		for _, h := range history[:n] {
			sum += h.FeelsMax
		}
		weekAvg := sum / float64(n)
		if d := today.FeelsMax - weekAvg; math.Abs(d) >= 2 {
			word := "warmer"
			if d < 0 {
				word = "colder"
			}
			out = append(out, fmt.Sprintf("%.0f° %s than the last %d days' average feels-like high (%.0f°).", math.Abs(d), word, n, weekAvg))
		}
	}
	return out
}

func headline(today weather.DaySummary, history []weather.DaySummary) string {
	h := fmt.Sprintf("%.0f–%.0f°C", today.TempMin, today.TempMax)
	if len(today.Conditions) > 0 {
		h += " " + today.Conditions[0]
	}
	if today.RainChance >= 30 {
		h += fmt.Sprintf(", rain %d%%", today.RainChance)
	}
	if len(history) > 0 && history[0].Date == dayBefore(today.Date) {
		d := today.FeelsMax - history[0].FeelsMax
		if d <= -3 {
			h += ", colder than yesterday"
		} else if d >= 3 {
			h += ", warmer than yesterday"
		}
	}
	return h
}
