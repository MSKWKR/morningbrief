package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/text/encoding/traditionalchinese"
)

const articleHTML = `<html><head><title>T</title></head><body><article><h1>T</h1><p>%s</p><p>Second paragraph with plenty of additional words so that the extraction heuristics keep this block as the main content of the page.</p></article></body></html>`

const bodyPara = "The quick brown fox jumps over the lazy dog while the readers watch with great interest and curiosity about the unfolding story."

// allowPorts permits only the listed loopback test-server ports.
func allowPorts(srvs ...*httptest.Server) Option {
	ports := map[uint16]bool{}
	for _, s := range srvs {
		ap := netip.MustParseAddrPort(strings.TrimPrefix(s.URL, "http://"))
		ports[ap.Port()] = true
	}
	return WithDialPolicy(func(ap netip.AddrPort) error {
		if ports[ap.Port()] {
			return nil
		}
		return fmt.Errorf("%w: test policy", ErrBlockedAddress)
	})
}

// site serves robots (404 unless given) and the article handler.
func site(robots string, article http.HandlerFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		if robots == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, robots)
	})
	mux.HandleFunc("/", article)
	return httptest.NewServer(mux)
}

func htmlHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, body)
	}
}

func TestTextHappyAndFixture(t *testing.T) {
	fx, err := os.ReadFile("testdata/article.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := site("", htmlHandler(string(fx)))
	defer srv.Close()
	got, err := New(allowPorts(srv)).Text(context.Background(), srv.URL+"/a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "quick brown fox") {
		t.Errorf("missing body paragraph: %q", got)
	}
	if strings.Contains(got, "SECRET_SCRIPT_TOKEN") {
		t.Errorf("script content leaked: %q", got)
	}
}

func TestDefaultPolicyBlocksLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := site("", func(w http.ResponseWriter, r *http.Request) { hits.Add(1) })
	defer srv.Close()
	_, err := New().Text(context.Background(), srv.URL+"/a")
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
	if hits.Load() != 0 {
		t.Fatal("server was reached")
	}
	// Robots failure hides the dial error; also check the article dial directly.
	f := New()
	req, _ := http.NewRequest("GET", srv.URL+"/a", nil)
	_, err = f.client.Do(req)
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("direct dial err = %v, want ErrBlockedAddress", err)
	}
}

func TestRedirectToOtherServerBlocked(t *testing.T) {
	var hitsB atomic.Int32
	b := site("", func(w http.ResponseWriter, r *http.Request) { hitsB.Add(1); htmlHandler("x")(w, r) })
	defer b.Close()
	a := site("", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, b.URL+"/x", http.StatusFound) })
	defer a.Close()
	_, err := New(allowPorts(a), WithRetryDelay(0)).Text(context.Background(), a.URL+"/a")
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v", err)
	}
	if hitsB.Load() != 0 {
		t.Fatal("redirect target reached")
	}
}

func TestTooManyRedirects(t *testing.T) {
	var hits atomic.Int32
	srv := site("", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/r%d", hits.Load()), http.StatusFound)
	})
	defer srv.Close()
	_, err := New(allowPorts(srv), WithRetryDelay(0)).Text(context.Background(), srv.URL+"/a")
	if !errors.Is(err, errRedirect) {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != maxRedirects+1 {
		t.Errorf("hits = %d, want %d (no retry)", hits.Load(), maxRedirects+1)
	}
}

func TestRedirectBadScheme(t *testing.T) {
	for _, target := range []string{"file:///etc/passwd", "ftp://example.com/x"} {
		srv := site("", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target, http.StatusFound) })
		_, err := New(allowPorts(srv), WithRetryDelay(0)).Text(context.Background(), srv.URL+"/a")
		srv.Close()
		if err == nil {
			t.Errorf("%s: expected error", target)
		}
	}
}

func TestBadURLs(t *testing.T) {
	f := New()
	for _, u := range []string{"http://user:pw@example.com/", "file:///etc/passwd", "ftp://example.com/", "http:///x", "://bad"} {
		if _, err := f.Text(context.Background(), u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

func TestTooLarge(t *testing.T) {
	srv := site("", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><p>"))
		w.Write([]byte(strings.Repeat("a", 6<<20)))
	})
	defer srv.Close()
	_, err := New(allowPorts(srv)).Text(context.Background(), srv.URL+"/a")
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestContentType(t *testing.T) {
	srv := site("", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.Write([]byte("%PDF"))
	})
	defer srv.Close()
	_, err := New(allowPorts(srv)).Text(context.Background(), srv.URL+"/a")
	if !errors.Is(err, ErrContentType) {
		t.Fatalf("err = %v", err)
	}
}

func TestStatus403NoRetry(t *testing.T) {
	var hits atomic.Int32
	srv := site("", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "secret body", http.StatusForbidden)
	})
	defer srv.Close()
	_, err := New(allowPorts(srv), WithRetryDelay(0)).Text(context.Background(), srv.URL+"/a")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 403 {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "secret") {
		t.Errorf("bad error string %q", err)
	}
	if hits.Load() != 1 {
		t.Errorf("hits = %d, want 1", hits.Load())
	}
}

func TestRetry503Then200(t *testing.T) {
	var hits atomic.Int32
	srv := site("", func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		htmlHandler(fmt.Sprintf(articleHTML, bodyPara))(w, r)
	})
	defer srv.Close()
	got, err := New(allowPorts(srv), WithRetryDelay(0)).Text(context.Background(), srv.URL+"/a")
	if err != nil || !strings.Contains(got, "quick brown fox") {
		t.Fatalf("got %q, err %v", got, err)
	}
	if hits.Load() != 2 {
		t.Errorf("hits = %d, want 2", hits.Load())
	}
}

func TestRetryRespectsContext(t *testing.T) {
	srv := site("", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 503) })
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(allowPorts(srv)).Text(ctx, srv.URL+"/a")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRobotsDisallow(t *testing.T) {
	var hits atomic.Int32
	srv := site("User-agent: *\nDisallow: /\n", func(w http.ResponseWriter, r *http.Request) { hits.Add(1) })
	defer srv.Close()
	_, err := New(allowPorts(srv)).Text(context.Background(), srv.URL+"/a")
	if !errors.Is(err, ErrDisallowedByRobots) {
		t.Fatalf("err = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("article handler hit")
	}
}

func TestRobots404Allowed(t *testing.T) {
	srv := site("", htmlHandler(fmt.Sprintf(articleHTML, bodyPara)))
	defer srv.Close()
	if _, err := New(allowPorts(srv)).Text(context.Background(), srv.URL+"/a"); err != nil {
		t.Fatal(err)
	}
}

func TestRobots5xxDisallows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.Error(w, "x", 500)
			return
		}
		htmlHandler("x")(w, r)
	}))
	defer srv.Close()
	_, err := New(allowPorts(srv)).Text(context.Background(), srv.URL+"/a")
	if !errors.Is(err, ErrDisallowedByRobots) {
		t.Fatalf("err = %v", err)
	}
}

func TestBig5(t *testing.T) {
	const zh = "這是一篇用來測試編碼的文章，內容包含足夠多的繁體中文字詞，讓可讀性演算法能夠辨識主要內容。"
	page := fmt.Sprintf(articleHTML, zh)
	enc, err := traditionalchinese.Big5.NewEncoder().String(page)
	if err != nil {
		t.Fatal(err)
	}
	srv := site("", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=big5")
		w.Write([]byte(enc))
	})
	defer srv.Close()
	got, err := New(allowPorts(srv)).Text(context.Background(), srv.URL+"/a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "測試編碼的文章") {
		t.Errorf("big5 not decoded: %q", got)
	}
}
