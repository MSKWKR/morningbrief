// Package fetch downloads untrusted web pages and extracts their readable
// text. The URLs come from attacker-influenced sources, so the HTTP client is
// SSRF-hardened: the IP policy is enforced in net.Dialer.Control on the
// address actually dialed, there is no proxy, no cookie jar and no credentials.
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/temoto/robotstxt"
	"golang.org/x/net/html/charset"
)

const (
	userAgent    = "morningbrief/0.1 (personal news digest)"
	maxBodyBytes = 5 << 20
	maxRedirects = 5
)

var (
	ErrTooLarge    = errors.New("fetch: body too large")
	ErrNoContent   = errors.New("fetch: no readable content")
	ErrContentType = errors.New("fetch: unsupported content type")
	errBadURL      = errors.New("fetch: invalid URL")
	errRedirect    = errors.New("fetch: redirect refused")
)

// StatusError is returned for non-2xx responses. It never carries the body.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("fetch: HTTP status %d", e.Code) }

// Option configures a Fetcher.
type Option func(*Fetcher)

// WithDialPolicy replaces the dial policy (for tests only).
func WithDialPolicy(p func(netip.AddrPort) error) Option { return func(f *Fetcher) { f.policy = p } }

// WithRetryDelay sets the wait before the single retry.
func WithRetryDelay(d time.Duration) Option { return func(f *Fetcher) { f.retryDelay = d } }

// Fetcher fetches pages through a private hardened client.
type Fetcher struct {
	policy     func(netip.AddrPort) error
	retryDelay time.Duration
	timeout    time.Duration
	client     *http.Client

	mu     sync.Mutex
	robots map[string]*robotstxt.RobotsData
}

// New builds a Fetcher with safe defaults.
func New(opts ...Option) *Fetcher {
	f := &Fetcher{
		policy:     defaultDialPolicy,
		retryDelay: 10 * time.Second,
		timeout:    30 * time.Second,
		robots:     map[string]*robotstxt.RobotsData{},
	}
	for _, o := range opts {
		o(f)
	}
	tr := &http.Transport{
		// Proxy must stay nil: with an env proxy the dial goes to the proxy
		// and the destination would never be checked.
		Proxy:                  nil,
		DialContext:            newDialer(f.policy).DialContext,
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  15 * time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		MaxIdleConns:           10,
		IdleConnTimeout:        30 * time.Second,
	}
	f.client = &http.Client{
		Transport: tr,
		// No Jar: cookies are never stored or sent.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("%w: too many redirects", errRedirect)
			}
			if err := validateURL(req.URL); err != nil {
				return fmt.Errorf("%w: %v", errRedirect, err)
			}
			return nil
		},
	}
	return f
}

func validateURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q", errBadURL, u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("%w: userinfo not allowed", errBadURL)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("%w: empty host", errBadURL)
	}
	return nil
}

// Text fetches rawURL and returns its raw extracted text. Sanitizing is the
// caller's job.
func (f *Fetcher) Text(ctx context.Context, rawURL string) (string, error) {
	return do(ctx, f, rawURL, f.attempt)
}

// do validates rawURL, checks robots.txt, then runs attempt, retrying once
// after retryDelay if the failure looks transient. It is shared by every kind
// of fetch so they all get the same URL, robots and retry rules.
func do[T any](ctx context.Context, f *Fetcher, rawURL string, attempt func(context.Context, *url.URL) (T, error)) (T, error) {
	var zero T
	u, err := url.Parse(rawURL)
	if err != nil {
		return zero, fmt.Errorf("%w: unparsable", errBadURL)
	}
	if err := validateURL(u); err != nil {
		return zero, err
	}
	ok, err := f.allowed(ctx, u)
	if err != nil {
		return zero, err
	}
	if !ok {
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		return zero, ErrDisallowedByRobots
	}

	res, err := attempt(ctx, u)
	if err == nil || !transient(err) {
		return res, err
	}
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-time.After(f.retryDelay):
	}
	return attempt(ctx, u)
}

// transient reports whether err merits the single retry.
func transient(err error) bool {
	if errors.Is(err, ErrBlockedAddress) || errors.Is(err, errRedirect) ||
		errors.Is(err, errBadURL) || errors.Is(err, context.Canceled) {
		return false
	}
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code == http.StatusTooManyRequests || se.Code >= 500
	}
	var ne net.Error
	var ue *url.Error
	return errors.As(err, &ne) || errors.As(err, &ue) || errors.Is(err, context.DeadlineExceeded)
}

func (f *Fetcher) attempt(ctx context.Context, u *url.URL) (string, error) {
	data, ct, final, err := f.getBody(ctx, u, "text/html,application/xhtml+xml",
		func(mt string) bool { return mt == "text/html" || mt == "application/xhtml+xml" })
	if err != nil {
		return "", err
	}
	r, err := charset.NewReader(bytes.NewReader(data), ct)
	if err != nil {
		return "", fmt.Errorf("fetch: charset: %w", err)
	}
	// Use the final URL (after redirects) for resolving relative links.
	article, err := readability.FromReader(r, final)
	if err != nil {
		return "", fmt.Errorf("fetch: extract: %w", err)
	}
	if article.Node == nil {
		return "", ErrNoContent
	}
	var sb strings.Builder
	if err := article.RenderText(&sb); err != nil {
		return "", fmt.Errorf("fetch: render: %w", err)
	}
	if strings.TrimSpace(sb.String()) == "" {
		return "", ErrNoContent
	}
	return sb.String(), nil
}
