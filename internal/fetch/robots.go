package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/temoto/robotstxt"
)

const (
	robotsAgent    = "morningbrief"
	maxRobotsBytes = 512 << 10
)

// ErrDisallowedByRobots is returned when robots.txt forbids fetching the URL.
var ErrDisallowedByRobots = errors.New("fetch: disallowed by robots.txt")

// allowed reports whether u may be fetched. Failures to obtain robots.txt
// mean "disallow" (RFC 9309 2.3.1.4); 4xx means allow all and 5xx disallow
// all, as implemented by robotstxt.FromStatusAndBytes. The one exception is a
// dial refused by the address policy: that is returned as ErrBlockedAddress so
// SSRF attempts are reported accurately (the article is still not requested).
func (f *Fetcher) allowed(ctx context.Context, u *url.URL) (bool, error) {
	key := u.Scheme + "://" + u.Host
	f.mu.Lock()
	rd, ok := f.robots[key]
	f.mu.Unlock()
	if !ok {
		var err error
		rd, err = f.fetchRobots(ctx, key)
		if err != nil {
			return false, err // blocked address: not cached
		}
		if ctx.Err() != nil {
			return false, nil // do not cache a result caused by cancellation
		}
		f.mu.Lock()
		f.robots[key] = rd
		f.mu.Unlock()
	}
	if rd == nil {
		return false, nil
	}
	return rd.TestAgent(u.RequestURI(), robotsAgent), nil
}

func (f *Fetcher) fetchRobots(ctx context.Context, origin string) (*robotstxt.RobotsData, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/robots.txt", nil)
	if err != nil {
		return nil, nil
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrBlockedAddress) {
			return nil, err
		}
		return nil, nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRobotsBytes))
	if err != nil {
		return nil, nil
	}
	rd, err := robotstxt.FromStatusAndBytes(resp.StatusCode, body)
	if err != nil {
		return nil, nil
	}
	return rd, nil
}
