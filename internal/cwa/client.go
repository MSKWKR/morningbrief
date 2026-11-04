// Package cwa talks to Taiwan's Central Weather Administration (中央氣象署)
// open data API: https://opendata.cwa.gov.tw
//
// We use the "鄉鎮天氣預報 未來3天" datasets (F-D0047-xxx), which give a
// district-level forecast in 1–3 hour steps. CWA does not forecast below
// district level, so a 里 like 葫洲里 uses its district (內湖區).
package cwa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const DefaultBaseURL = "https://opendata.cwa.gov.tw/api/v1/rest/datastore/"

// Response mirrors the parts of the JSON we care about. Struct tags tell
// encoding/json which JSON key maps to which field.
type Response struct {
	Success string `json:"success"`
	Records struct {
		Locations []struct {
			LocationsName string     `json:"LocationsName"` // city, e.g. 臺北市
			Location      []Location `json:"Location"`
		} `json:"Locations"`
	} `json:"records"`
}

type Location struct {
	LocationName   string    `json:"LocationName"` // district, e.g. 內湖區
	Latitude       string    `json:"Latitude"`
	Longitude      string    `json:"Longitude"`
	WeatherElement []Element `json:"WeatherElement"`
}

// Element is one forecast variable (溫度, 體感溫度, 3小時降雨機率 ...).
type Element struct {
	ElementName string      `json:"ElementName"`
	Time        []TimeEntry `json:"Time"`
}

// TimeEntry is either a point in time (DataTime) or an interval
// (StartTime/EndTime), depending on the element. ElementValue holds
// key/value pairs like {"Temperature": "23"}.
type TimeEntry struct {
	DataTime     string              `json:"DataTime,omitempty"`
	StartTime    string              `json:"StartTime,omitempty"`
	EndTime      string              `json:"EndTime,omitempty"`
	ElementValue []map[string]string `json:"ElementValue"`
}

type Client struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

// TownshipForecast fetches one district from a F-D0047 dataset.
// It returns the parsed location plus the raw body so it can be archived.
func (c *Client) TownshipForecast(ctx context.Context, datasetID, district string) (*Location, []byte, error) {
	q := url.Values{}
	q.Set("Authorization", c.APIKey)
	q.Set("LocationName", district)
	q.Set("format", "JSON")
	q.Set("sort", "time")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+datasetID+"?"+q.Encode(), nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("cwa request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("cwa read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, body, fmt.Errorf("cwa: HTTP %d: %.200s", resp.StatusCode, body)
	}

	loc, err := Parse(body, district)
	return loc, body, err
}

// Parse decodes a F-D0047 response and returns the named district.
// It is separate from the HTTP call so tests (and -fixture runs) can use it.
func Parse(body []byte, district string) (*Location, error) {
	var r Response
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("cwa decode: %w", err)
	}
	if r.Success != "true" {
		return nil, fmt.Errorf("cwa: success=%q", r.Success)
	}
	for _, city := range r.Records.Locations {
		for i := range city.Location {
			if city.Location[i].LocationName == district {
				return &city.Location[i], nil
			}
		}
	}
	return nil, fmt.Errorf("cwa: district %q not in response", district)
}
