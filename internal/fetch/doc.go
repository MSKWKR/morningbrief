package fetch

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
)

// DocKind selects which kind of non-HTML document Doc will accept.
type DocKind int

const (
	// Feed is an RSS or Atom document.
	Feed DocKind = iota
	// JSON is a JSON document.
	JSON
)

// Doc fetches an RSS/Atom feed or a JSON document and returns its raw bytes.
// It goes through exactly the same guarded path as Text: URL validation,
// robots.txt, the SSRF dial policy, the redirect rules, the size cap and the
// single retry. The caller must treat the bytes as untrusted.
func (f *Fetcher) Doc(ctx context.Context, rawURL string, kind DocKind) ([]byte, error) {
	accept, okType := docType(kind)
	return do(ctx, f, rawURL, func(ctx context.Context, u *url.URL) ([]byte, error) {
		data, _, _, err := f.getBody(ctx, u, accept, okType)
		return data, err
	})
}

func docType(kind DocKind) (accept string, ok func(string) bool) {
	switch kind {
	case JSON:
		return "application/json", func(mt string) bool {
			return mt == "application/json" || mt == "text/json"
		}
	default:
		return "application/rss+xml, application/atom+xml, application/xml, text/xml", func(mt string) bool {
			switch mt {
			case "application/rss+xml", "application/atom+xml", "application/xml", "text/xml", "application/rdf+xml":
				return true
			}
			return false
		}
	}
}

// getBody performs one guarded GET and returns the body, its Content-Type
// header, and the final URL after redirects. okType decides which media types
// are acceptable.
func (f *Fetcher) getBody(ctx context.Context, u *url.URL, accept string, okType func(string) bool) ([]byte, string, *url.URL, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", nil, fmt.Errorf("%w: %v", errBadURL, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	// Accept-Encoding is deliberately unset so Go decompresses transparently
	// and the size cap below bounds the decompressed bytes.

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", nil, &StatusError{Code: resp.StatusCode}
	}
	ct := resp.Header.Get("Content-Type")
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil || !okType(mt) {
		return nil, "", nil, fmt.Errorf("%w: %q", ErrContentType, mt)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, "", nil, err
	}
	if len(data) > maxBodyBytes {
		return nil, "", nil, ErrTooLarge
	}
	return data, ct, resp.Request.URL, nil
}
