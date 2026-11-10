// Package weather turns a raw CWA forecast into a compact DaySummary,
// which is what we store and compare day to day.
package weather

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"morningbrief/internal/cwa"
)

// Period is a slice of the day, e.g. 早上 06–12.
type Period struct {
	Name       string  `json:"name"`
	TempMin    float64 `json:"temp_min"`
	TempMax    float64 `json:"temp_max"`
	FeelsMin   float64 `json:"feels_min"`
	FeelsMax   float64 `json:"feels_max"`
	RainChance int     `json:"rain_chance"` // max % in the period
	Condition  string  `json:"condition"`   // most common 天氣現象
}

// DaySummary is the per-day record saved to disk.
type DaySummary struct {
	Date        string    `json:"date"` // YYYY-MM-DD, Asia/Taipei
	Place       string    `json:"place"`
	FetchedAt   time.Time `json:"fetched_at"`
	TempMin     float64   `json:"temp_min"`
	TempMax     float64   `json:"temp_max"`
	FeelsMin    float64   `json:"feels_min"`
	FeelsMax    float64   `json:"feels_max"`
	RainChance  int       `json:"rain_chance"` // max % over the day
	HumidityAvg int       `json:"humidity_avg"`
	WindMax     int       `json:"wind_max_beaufort"`
	Conditions  []string  `json:"conditions"` // distinct, in time order
	Periods     []Period  `json:"periods"`
}

// The day window we care about: 06:00 to midnight.
var periodDefs = []struct {
	name       string
	start, end int // hours
}{
	{"早上 06–12", 6, 12},
	{"下午 12–18", 12, 18},
	{"晚上 18–24", 18, 24},
}

type point struct {
	t time.Time
	v float64
}

type interval struct {
	start, end time.Time
	v          float64
	text       string
}

// Summarize builds the summary for the calendar day containing `day`
// (interpreted in day's location, which should be Asia/Taipei).
func Summarize(loc *cwa.Location, day time.Time, place string) DaySummary {
	y, m, d := day.Date()
	dayStart := time.Date(y, m, d, 6, 0, 0, 0, day.Location())
	dayEnd := time.Date(y, m, d, 0, 0, 0, 0, day.Location()).AddDate(0, 0, 1)

	temp := points(loc, "溫度", "Temperature")
	feels := points(loc, "體感溫度", "ApparentTemperature")
	rh := points(loc, "相對濕度", "RelativeHumidity")
	wind := points(loc, "風速", "BeaufortScale")
	pop := intervals(loc, "3小時降雨機率", "ProbabilityOfPrecipitation")
	wx := intervals(loc, "天氣現象", "Weather")

	s := DaySummary{
		Date:      day.Format("2006-01-02"),
		Place:     place,
		FetchedAt: time.Now().In(day.Location()),
	}
	s.TempMin, s.TempMax = minMax(between(temp, dayStart, dayEnd))
	s.FeelsMin, s.FeelsMax = minMax(between(feels, dayStart, dayEnd))
	s.HumidityAvg = int(math.Round(avg(between(rh, dayStart, dayEnd))))
	_, w := minMax(between(wind, dayStart, dayEnd))
	s.WindMax = int(w)
	s.RainChance = int(maxOverlap(pop, dayStart, dayEnd))
	s.Conditions = distinctTexts(wx, dayStart, dayEnd)

	for _, p := range periodDefs {
		ps := time.Date(y, m, d, p.start, 0, 0, 0, day.Location())
		pe := time.Date(y, m, d, 0, 0, 0, 0, day.Location()).Add(time.Duration(p.end) * time.Hour)
		tp := between(temp, ps, pe)
		if len(tp) == 0 {
			continue // e.g. forecast already past this period
		}
		per := Period{Name: p.name}
		per.TempMin, per.TempMax = minMax(tp)
		per.FeelsMin, per.FeelsMax = minMax(between(feels, ps, pe))
		per.RainChance = int(maxOverlap(pop, ps, pe))
		per.Condition = mostCommon(wx, ps, pe)
		s.Periods = append(s.Periods, per)
	}
	return s
}

func findElement(loc *cwa.Location, name string) *cwa.Element {
	for i := range loc.WeatherElement {
		if loc.WeatherElement[i].ElementName == name {
			return &loc.WeatherElement[i]
		}
	}
	return nil
}

func value(te cwa.TimeEntry, key string) (string, bool) {
	for _, ev := range te.ElementValue {
		if v, ok := ev[key]; ok {
			return v, true
		}
	}
	return "", false
}

func points(loc *cwa.Location, element, key string) []point {
	el := findElement(loc, element)
	if el == nil {
		return nil
	}
	var out []point
	for _, te := range el.Time {
		ts := te.DataTime
		if ts == "" {
			ts = te.StartTime
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		raw, _ := value(te, key)
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			continue // CWA uses "-" or "" for missing values
		}
		out = append(out, point{t, v})
	}
	return out
}

func intervals(loc *cwa.Location, element, key string) []interval {
	el := findElement(loc, element)
	if el == nil {
		return nil
	}
	var out []interval
	for _, te := range el.Time {
		st, err1 := time.Parse(time.RFC3339, te.StartTime)
		et, err2 := time.Parse(time.RFC3339, te.EndTime)
		if err1 != nil || err2 != nil {
			continue
		}
		raw, _ := value(te, key)
		v, _ := strconv.ParseFloat(raw, 64)
		out = append(out, interval{st, et, v, raw})
	}
	return out
}

func between(ps []point, start, end time.Time) []float64 {
	var out []float64
	for _, p := range ps {
		if !p.t.Before(start) && p.t.Before(end) {
			out = append(out, p.v)
		}
	}
	return out
}

func overlapping(is []interval, start, end time.Time) []interval {
	var out []interval
	for _, iv := range is {
		if iv.start.Before(end) && iv.end.After(start) {
			out = append(out, iv)
		}
	}
	return out
}

func maxOverlap(is []interval, start, end time.Time) float64 {
	m := 0.0
	for _, iv := range overlapping(is, start, end) {
		m = math.Max(m, iv.v)
	}
	return m
}

func distinctTexts(is []interval, start, end time.Time) []string {
	seen := map[string]bool{}
	var out []string
	for _, iv := range overlapping(is, start, end) {
		if iv.text != "" && !seen[iv.text] {
			seen[iv.text] = true
			out = append(out, iv.text)
		}
	}
	return out
}

func mostCommon(is []interval, start, end time.Time) string {
	count := map[string]int{}
	best, bestN := "", 0
	for _, iv := range overlapping(is, start, end) {
		count[iv.text]++
		if count[iv.text] > bestN {
			best, bestN = iv.text, count[iv.text]
		}
	}
	return best
}

func minMax(vs []float64) (float64, float64) {
	if len(vs) == 0 {
		return 0, 0
	}
	lo, hi := vs[0], vs[0]
	for _, v := range vs[1:] {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	return lo, hi
}

func avg(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}

// TempRange prints "19" when low and high round the same, else "18–24".
func TempRange(lo, hi float64) string {
	a, b := fmt.Sprintf("%.0f", lo), fmt.Sprintf("%.0f", hi)
	if a == b {
		return a
	}
	return a + "–" + b
}
