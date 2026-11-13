package feed

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const rssSample = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><title>Example</title>
<item>
  <title><![CDATA[Ransomware gang "Qilin" & friends]]></title>
  <link>https://example.com/a?x=1&amp;y=2</link>
  <description><![CDATA[<p>First <b>para</b> &amp; more.</p><script>alert(1)</script>]]></description>
  <pubDate>Fri, 13 Nov 2026 17:22:31 +0530</pubDate>
</item>
<item>
  <title>Dated by dc</title>
  <link>https://example.com/b</link>
  <description>Plain description</description>
  <dc:date>2026-11-12T10:00:00Z</dc:date>
</item>
<item><title>No date</title><link>https://example.com/c</link></item>
</channel></rss>`

func TestParseRSS(t *testing.T) {
	es, err := Parse([]byte(rssSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 3 {
		t.Fatalf("got %d entries", len(es))
	}
	if es[0].Title != `Ransomware gang "Qilin" & friends` {
		t.Errorf("title = %q", es[0].Title)
	}
	if es[0].Link != "https://example.com/a?x=1&y=2" {
		t.Errorf("link = %q", es[0].Link)
	}
	// Descriptions are returned raw: HTML handling is the caller's job.
	if !strings.Contains(es[0].Summary, "<script>") {
		t.Errorf("summary should be raw, got %q", es[0].Summary)
	}
	want := time.Date(2026, 11, 13, 11, 52, 31, 0, time.UTC)
	if !es[0].Published.Equal(want) {
		t.Errorf("published = %v, want %v", es[0].Published, want)
	}
	if want := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC); !es[1].Published.Equal(want) {
		t.Errorf("dc:date = %v, want %v", es[1].Published, want)
	}
	if !es[2].Published.IsZero() {
		t.Errorf("missing date should be zero, got %v", es[2].Published)
	}
}

const atomSample = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Atom</title>
<entry>
  <title>Atom one</title>
  <link rel="enclosure" href="https://example.com/file.mp3"/>
  <link rel="alternate" href="https://example.com/atom-one"/>
  <summary>Short summary</summary>
  <updated>2026-11-12T08:30:00+00:00</updated>
</entry>
<entry>
  <title>Atom two</title>
  <link href="https://example.com/atom-two"/>
  <content type="html">&lt;p&gt;Body&lt;/p&gt;</content>
  <published>2026-11-11T08:30:00Z</published>
  <updated>2026-11-12T09:00:00Z</updated>
</entry>
</feed>`

func TestParseAtom(t *testing.T) {
	es, err := Parse([]byte(atomSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 {
		t.Fatalf("got %d entries", len(es))
	}
	if es[0].Link != "https://example.com/atom-one" {
		t.Errorf("link = %q (should skip the enclosure)", es[0].Link)
	}
	if es[1].Link != "https://example.com/atom-two" || es[1].Summary != "<p>Body</p>" {
		t.Errorf("entry 2 = %+v", es[1])
	}
	// published wins over updated.
	if want := time.Date(2026, 11, 11, 8, 30, 0, 0, time.UTC); !es[1].Published.Equal(want) {
		t.Errorf("published = %v, want %v", es[1].Published, want)
	}
}

func TestParseCharsetAndErrors(t *testing.T) {
	// ISO-8859-1 declared, with a byte that is invalid UTF-8.
	latin := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><rss><channel><item><title>Caf\xe9</title><link>https://e.example/x</link></item></channel></rss>"
	es, err := Parse([]byte(latin))
	if err != nil || len(es) != 1 || es[0].Title != "Café" {
		t.Fatalf("latin1: %+v, %v", es, err)
	}

	if _, err := Parse([]byte(`<rss><channel></channel></rss>`)); !errors.Is(err, ErrNoEntries) {
		t.Errorf("empty feed err = %v, want ErrNoEntries", err)
	}
	for _, bad := range []string{"", "not xml at all", "<rss><channel><item>", `{"json": true}`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("Parse(%q) succeeded", bad)
		}
	}
}

func TestParseDoesNotExpandEntities(t *testing.T) {
	// A DTD-defined entity must not be expanded (no billion-laughs / XXE).
	doc := `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY a "AAAA"><!ENTITY file SYSTEM "file:///etc/passwd">]>
<rss><channel><item><title>&a;&file;</title><link>https://e.example/x</link></item></channel></rss>`
	es, err := Parse([]byte(doc))
	if err == nil && len(es) == 1 && (strings.Contains(es[0].Title, "AAAA") || strings.Contains(es[0].Title, "root:")) {
		t.Errorf("entity was expanded: %q", es[0].Title)
	}
}

func TestParseTime(t *testing.T) {
	for _, s := range []string{
		"Fri, 13 Nov 2026 17:22:31 +0530", "Fri, 13 Nov 2026 12:44:20 GMT", "Thu, 12 Nov 2026 20:22:00 -0500",
		"Fri, 6 Nov 2026 01:02:03 +0000", "2026-11-12T10:00:00Z", "2026-11-12T10:00:00+08:00",
	} {
		if parseTime(s).IsZero() {
			t.Errorf("parseTime(%q) failed", s)
		}
	}
	for _, s := range []string{"", "yesterday", "13/11/2026"} {
		if !parseTime(s).IsZero() {
			t.Errorf("parseTime(%q) should fail", s)
		}
	}
}
