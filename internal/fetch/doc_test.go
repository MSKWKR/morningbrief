package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func typed(ct, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ct)
		fmt.Fprint(w, body)
	}
}

func TestDocFeedAndJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind DocKind
		ct   string
		ok   bool
	}{
		{"rss", Feed, "application/rss+xml; charset=UTF-8", true},
		{"atom", Feed, "application/atom+xml", true},
		{"xml", Feed, "application/xml", true},
		{"text xml", Feed, "text/xml; charset=utf-8", true},
		{"html is not a feed", Feed, "text/html", false},
		{"json is not a feed", Feed, "application/json", false},
		{"json", JSON, "application/json", true},
		{"json with charset", JSON, "application/json; charset=utf-8", true},
		{"xml is not json", JSON, "application/xml", false},
		{"html is not json", JSON, "text/html", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := site("", typed(tc.ct, "<ok/>"))
			defer srv.Close()
			f := New(allowPorts(srv), WithRetryDelay(0))
			got, err := f.Doc(context.Background(), srv.URL+"/x", tc.kind)
			if tc.ok {
				if err != nil || string(got) != "<ok/>" {
					t.Fatalf("got %q, %v", got, err)
				}
				return
			}
			if !errors.Is(err, ErrContentType) {
				t.Fatalf("err = %v, want ErrContentType", err)
			}
		})
	}
}

func TestDocGetsSameProtections(t *testing.T) {
	// Default policy: loopback is blocked for Doc exactly as for Text, and the
	// document handler is never reached.
	var hits atomic.Int32
	srv := site("", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		typed("application/xml", "<ok/>")(w, r)
	})
	defer srv.Close()
	if _, err := New().Doc(context.Background(), srv.URL+"/x", Feed); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
	if hits.Load() != 0 {
		t.Errorf("document handler hit %d times", hits.Load())
	}

	// robots.txt applies to documents too.
	srv2 := site("User-agent: *\nDisallow: /", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		typed("application/xml", "<ok/>")(w, r)
	})
	defer srv2.Close()
	if _, err := New(allowPorts(srv2)).Doc(context.Background(), srv2.URL+"/feed", Feed); !errors.Is(err, ErrDisallowedByRobots) {
		t.Fatalf("err = %v, want ErrDisallowedByRobots", err)
	}
	if hits.Load() != 0 {
		t.Errorf("document handler hit despite robots.txt: %d", hits.Load())
	}

	// Bad scheme and userinfo are rejected before any request.
	for _, u := range []string{"file:///etc/passwd", "ftp://example.com/x", "http://user:pw@example.com/x"} {
		if _, err := New().Doc(context.Background(), u, Feed); err == nil {
			t.Errorf("Doc(%q) succeeded", u)
		}
	}
}

func TestDocSizeAndStatus(t *testing.T) {
	big := site("", typed("application/xml", strings.Repeat("a", 6<<20)))
	defer big.Close()
	if _, err := New(allowPorts(big)).Doc(context.Background(), big.URL+"/x", Feed); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}

	var hits atomic.Int32
	forbidden := site("", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "secret-body", http.StatusForbidden)
	})
	defer forbidden.Close()
	_, err := New(allowPorts(forbidden), WithRetryDelay(0)).Doc(context.Background(), forbidden.URL+"/x", Feed)
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 403 {
		t.Fatalf("err = %v, want StatusError 403", err)
	}
	if strings.Contains(err.Error(), "secret-body") {
		t.Errorf("error leaks body: %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("403 was retried: %d hits", hits.Load())
	}
}
