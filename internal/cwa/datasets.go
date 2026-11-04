package cwa

import (
	"fmt"
	"strings"
)

// threeDayDatasets maps each city/county to its 3-day township forecast
// dataset ID (鄉鎮天氣預報 未來3天). The 1-week datasets are these IDs + 2.
var threeDayDatasets = map[string]string{
	"宜蘭縣": "F-D0047-001",
	"桃園市": "F-D0047-005",
	"新竹縣": "F-D0047-009",
	"苗栗縣": "F-D0047-013",
	"彰化縣": "F-D0047-017",
	"南投縣": "F-D0047-021",
	"雲林縣": "F-D0047-025",
	"嘉義縣": "F-D0047-029",
	"屏東縣": "F-D0047-033",
	"臺東縣": "F-D0047-037",
	"花蓮縣": "F-D0047-041",
	"澎湖縣": "F-D0047-045",
	"基隆市": "F-D0047-049",
	"新竹市": "F-D0047-053",
	"嘉義市": "F-D0047-057",
	"臺北市": "F-D0047-061",
	"高雄市": "F-D0047-065",
	"新北市": "F-D0047-069",
	"臺中市": "F-D0047-073",
	"臺南市": "F-D0047-077",
	"連江縣": "F-D0047-081",
	"金門縣": "F-D0047-085",
}

// NormalizeName converts 台 to 臺, which is what CWA uses (台北市 → 臺北市).
func NormalizeName(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "台", "臺")
}

// DatasetForCity returns the 3-day forecast dataset ID for a city/county.
func DatasetForCity(city string) (string, error) {
	if id, ok := threeDayDatasets[NormalizeName(city)]; ok {
		return id, nil
	}
	return "", fmt.Errorf("unknown city %q (use the full name, e.g. 臺北市, 新竹縣)", city)
}
