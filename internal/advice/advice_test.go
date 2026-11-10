package advice

import (
	"testing"

	"morningbrief/internal/weather"
)

func TestCompareWording(t *testing.T) {
	today := weather.DaySummary{Date: "2026-11-10", FeelsMax: 20, FeelsMin: 19}
	cases := []struct {
		name string
		prev weather.DaySummary
		want string
	}{
		{"colder, yesterday", weather.DaySummary{Date: "2026-11-09", FeelsMax: 27, FeelsMin: 26},
			"Today feels 7° colder than yesterday (high 20° vs 27°). Wear one more layer than yesterday."},
		{"warmer, older day uses short date", weather.DaySummary{Date: "2026-11-05", FeelsMax: 16, FeelsMin: 15},
			"Today feels 4° warmer than 11/05 (high 20° vs 16°). You can wear one layer less than on 11/05."},
		{"high and low moved differently", weather.DaySummary{Date: "2026-11-09", FeelsMax: 21, FeelsMin: 24},
			"Today feels 5° colder than yesterday (high 20° vs 21°, low 19° vs 24°). Wear one more layer than yesterday."},
		{"about the same", weather.DaySummary{Date: "2026-11-09", FeelsMax: 21, FeelsMin: 20},
			"Today feels about the same as yesterday (high 20° vs 21°). The same outfit works."},
	}
	for _, c := range cases {
		got := compare(today, []weather.DaySummary{c.prev})
		if len(got) == 0 || got[0] != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, got, c.want)
		}
	}
}
