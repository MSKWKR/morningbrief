package hn

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestQualifies(t *testing.T) {
	story := func(mod func(*Item)) Item {
		it := Item{ID: 1, Type: "story", Title: "T", URL: "https://example.com/a/b"}
		mod(&it)
		return it
	}
	tests := []struct {
		name string
		it   Item
		want bool
	}{
		{"normal article", story(func(*Item) {}), true},
		{"job", story(func(i *Item) { i.Type = "job" }), false},
		{"dead", story(func(i *Item) { i.Dead = true }), false},
		{"deleted", story(func(i *Item) { i.Deleted = true }), false},
		{"empty title", story(func(i *Item) { i.Title = "" }), false},
		{"pdf", story(func(i *Item) { i.URL = "https://example.com/paper.pdf" }), false},
		{"PDF upper", story(func(i *Item) { i.URL = "https://example.com/paper.PDF?x=1" }), false},
		{"youtube", story(func(i *Item) { i.URL = "https://www.youtube.com/watch?v=x" }), false},
		{"youtu.be", story(func(i *Item) { i.URL = "https://youtu.be/x" }), false},
		{"mobile vimeo", story(func(i *Item) { i.URL = "https://m.vimeo.com/1" }), false},
		{"upper host", story(func(i *Item) { i.URL = "https://YouTube.com/x" }), false},
		{"x.com", story(func(i *Item) { i.URL = "https://x.com/u/status/1" }), false},
		{"twitter", story(func(i *Item) { i.URL = "https://twitter.com/u" }), false},
		{"lookalike host", story(func(i *Item) { i.URL = "https://notyoutube.com/x" }), true},
		{"javascript url", story(func(i *Item) { i.URL = "javascript:alert(1)" }), false},
		{"ftp url", story(func(i *Item) { i.URL = "ftp://example.com/x" }), false},
		{"userinfo", story(func(i *Item) { i.URL = "https://user:pw@example.com/" }), false},
		{"no host", story(func(i *Item) { i.URL = "https:///path" }), false},
		{"text post with text", story(func(i *Item) { i.URL = ""; i.Text = "<p>Hello</p>" }), true},
		{"text post without text", story(func(i *Item) { i.URL = ""; i.Text = "" }), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Qualifies(tc.it); got != tc.want {
				t.Errorf("Qualifies = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDiscussionURL(t *testing.T) {
	if got, want := DiscussionURL(42), "https://news.ycombinator.com/item?id=42"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// newServer serves topstories and items; items maps id to raw body, and a
// missing id returns 500.
func newServer(t *testing.T, top string, items map[int]string) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/topstories.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, top)
	})
	mux.HandleFunc("/item/", func(w http.ResponseWriter, r *http.Request) {
		var id int
		fmt.Sscanf(strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/item/"), ".json"), "%d", &id)
		body, ok := items[id]
		if !ok {
			http.Error(w, "secret-body", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New()
	c.BaseURL = srv.URL
	c.Logf = func(string, ...any) {}
	return c
}

func storyJSON(id int) string {
	return fmt.Sprintf(`{"id":%d,"type":"story","title":"S%d","url":"https://example.com/%d","score":5,"descendants":2}`, id, id, id)
}

func TestSelect(t *testing.T) {
	items := map[int]string{
		1: storyJSON(1),
		2: `{"id":2,"type":"job","title":"Hiring","url":"https://example.com/j"}`,
		// 3 missing -> 500
		4: `null`,
		5: storyJSON(5),
		6: `{"id":6,"type":"story","title":"Vid","url":"https://youtu.be/x"}`,
		7: storyJSON(7),
		8: storyJSON(8),
		9: storyJSON(9),
	}
	c := newServer(t, `[1,2,3,4,5,6,7,8,9]`, items)
	var logged []string
	c.Logf = func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) }

	got, err := c.Select(context.Background(), SelectOptions{Scan: 30, Want: 3})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for _, it := range got {
		ids = append(ids, it.ID)
	}
	if want := []int{1, 5, 7}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if len(logged) != 2 {
		t.Errorf("logged %d skips, want 2: %v", len(logged), logged)
	}
	for _, l := range logged {
		if strings.Contains(l, "secret-body") {
			t.Errorf("log leaks body: %q", l)
		}
	}
}

func TestSelectScanLimit(t *testing.T) {
	items := map[int]string{1: storyJSON(1), 2: storyJSON(2), 3: storyJSON(3)}
	c := newServer(t, `[1,2,3]`, items)
	got, err := c.Select(context.Background(), SelectOptions{Scan: 2, Want: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("got %d items, want 2", len(got))
	}
}

func TestSelectTopStoriesFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom-body", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := New()
	c.BaseURL = srv.URL
	_, err := c.Select(context.Background(), SelectOptions{Scan: 30, Want: 10})
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err = %v, want status 502", err)
	}
	if strings.Contains(err.Error(), "boom-body") {
		t.Errorf("error leaks body: %v", err)
	}
}

func TestSelectCancelled(t *testing.T) {
	c := newServer(t, `[1,2]`, map[int]string{1: storyJSON(1), 2: storyJSON(2)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Select(ctx, SelectOptions{Scan: 30, Want: 10}); err == nil {
		t.Error("expected error when top-stories call is cancelled")
	}
}

func TestItemNull(t *testing.T) {
	c := newServer(t, `[]`, map[int]string{1: `null`})
	if _, err := c.Item(context.Background(), 1); err == nil {
		t.Error("expected error for null item")
	}
}

func TestItemDecode(t *testing.T) {
	c := newServer(t, `[]`, map[int]string{50439193: `{"by":"ilreb","descendants":57,"id":50439193,"kids":[1,2],"score":130,"time":1794312672,"title":"AlphaProof Code","type":"story","url":"https://deepmind.google/blog/x"}`})
	it, err := c.Item(context.Background(), 50439193)
	if err != nil {
		t.Fatal(err)
	}
	want := Item{ID: 50439193, Type: "story", By: "ilreb", Title: "AlphaProof Code", URL: "https://deepmind.google/blog/x", Score: 130, Time: 1794312672, Descendants: 57}
	if it != want {
		t.Errorf("got %+v want %+v", it, want)
	}
}

func TestOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("["))
		w.Write([]byte(strings.Repeat("1,", 600*1024)))
		w.Write([]byte("1]"))
	}))
	defer srv.Close()
	c := New()
	c.BaseURL = srv.URL
	_, err := c.TopStoryIDs(context.Background())
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

// testNow is the pretend clock for the selection tests.
var testNow = time.Date(2026, 11, 12, 7, 0, 0, 0, time.UTC)

// at builds a story submitted ago before testNow.
func at(id, score int, ago time.Duration, title, url string) string {
	return fmt.Sprintf(`{"id":%d,"type":"story","title":%q,"url":%q,"score":%d,"time":%d}`,
		id, title, url, score, testNow.Add(-ago).Unix())
}

func ids(items []Item) []int {
	var out []int
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestSelectFreshRankedByScore(t *testing.T) {
	items := map[int]string{
		1: at(1, 900, 60*time.Hour, "Days-old front page resident", "https://a.example/x"), // too old
		2: at(2, 150, 3*time.Hour, "New but low score", "https://b.example/x"),             // below MinScore
		3: at(3, 300, 10*time.Hour, "Fresh, mid score", "https://c.example/x"),
		4: at(4, 800, 20*time.Hour, "Fresh, high score", "https://d.example/x"),
		5: at(5, 300, 2*time.Hour, "Fresh, tied score, later in list", "https://e.example/x"),
		6: at(6, 400, 23*time.Hour+59*time.Minute, "Just inside the window", "https://f.example/x"),
		7: at(7, 400, 24*time.Hour+time.Minute, "Just outside the window", "https://g.example/x"),
	}
	c := newServer(t, `[1,2,3,4,5,6,7]`, items)
	got, err := c.Select(context.Background(), SelectOptions{Scan: 30, Want: 10, MaxAge: 24 * time.Hour, MinScore: 200, Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	// Highest score first; the 300-point tie keeps list order (3 before 5).
	if want := []int{4, 6, 3, 5}; !reflect.DeepEqual(ids(got), want) {
		t.Errorf("ids = %v, want %v", ids(got), want)
	}
}

func TestSelectWantAppliesAfterRanking(t *testing.T) {
	items := map[int]string{
		1: at(1, 250, time.Hour, "A", "https://a.example/x"),
		2: at(2, 900, time.Hour, "B", "https://b.example/x"),
		3: at(3, 500, time.Hour, "C", "https://c.example/x"),
	}
	c := newServer(t, `[1,2,3]`, items)
	got, err := c.Select(context.Background(), SelectOptions{Scan: 30, Want: 2, MaxAge: 24 * time.Hour, Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	// Want must trim to the best two, not the first two in list order.
	if want := []int{2, 3}; !reflect.DeepEqual(ids(got), want) {
		t.Errorf("ids = %v, want %v", ids(got), want)
	}
}

func TestSelectDropsOldLookingStories(t *testing.T) {
	items := map[int]string{
		1: at(1, 500, time.Hour, "A great old-timey hack (2013)", "https://blog.example/posts/x"),
		2: at(2, 500, time.Hour, "Resubmitted post", "https://blog.example/posts/2013-12-14-great-old-timey-hack.html"),
		3: at(3, 500, time.Hour, "Resubmitted post, slash dates", "https://blog.example/2019/03/01/x"),
		4: at(4, 500, time.Hour, "Last year is still fine (2025)", "https://blog.example/2025/12/31/x"),
		5: at(5, 500, time.Hour, "Current", "https://blog.example/2026/11/11/x"),
	}
	c := newServer(t, `[1,2,3,4,5]`, items)
	got, err := c.Select(context.Background(), SelectOptions{Scan: 30, Want: 10, MaxAge: 24 * time.Hour, Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int{4, 5}; !reflect.DeepEqual(ids(got), want) {
		t.Errorf("ids = %v, want %v", ids(got), want)
	}
}

func TestSelectMissingTimeNeedsNoLimit(t *testing.T) {
	// No time field: excluded when a max age is set, kept when it is not.
	items := map[int]string{1: storyJSON(1)}
	c := newServer(t, `[1]`, items)
	got, _ := c.Select(context.Background(), SelectOptions{Scan: 30, Want: 10, MaxAge: 24 * time.Hour, Now: testNow})
	if len(got) != 0 {
		t.Errorf("story without a timestamp passed a max-age filter: %v", ids(got))
	}
	got, _ = c.Select(context.Background(), SelectOptions{Scan: 30, Want: 10, Now: testNow})
	if len(got) != 1 {
		t.Errorf("story without a timestamp dropped with no max age: %v", ids(got))
	}
}

func TestLooksOld(t *testing.T) {
	for _, tc := range []struct {
		title, url string
		want       bool
	}{
		{"Plain title", "https://example.com/a", false},
		{"Foo (2013)", "https://example.com/a", true},
		{"Foo (2024)", "https://example.com/a", true},
		{"Foo (2025)", "https://example.com/a", false}, // last year allowed
		{"Foo (2026)", "https://example.com/a", false},
		{"Released in 2013", "https://example.com/a", false}, // only "(YEAR)" counts
		{"Plain", "https://example.com/2013/12/14/post", true},
		{"Plain", "https://example.com/posts/2013-12-14-title", true},
		{"Plain", "https://example.com/blog/2012-06", true},
		{"Plain", "https://example.com/2024/05/01/post", true},
		{"Plain", "https://example.com/2025/12/31/post", false},
		{"Plain", "https://example.com/2026/11/10/post", false},
		{"Plain", "https://arxiv.org/abs/2301.01234", false},
		{"Plain", "https://example.com/v2013/release", false},
		{"Plain", "https://example.com/2013", false}, // a lone year is not a date
		{"Plain", "", false}, // text posts have no URL
	} {
		if got := LooksOld(Item{Title: tc.title, URL: tc.url}, testNow); got != tc.want {
			t.Errorf("LooksOld(%q, %q) = %v, want %v", tc.title, tc.url, got, tc.want)
		}
	}
}
