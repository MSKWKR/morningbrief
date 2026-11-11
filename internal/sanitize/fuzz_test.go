package sanitize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzClean(f *testing.F) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.txt"))
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(b), uint16(10000))
		f.Add(string(b), uint16(7))
	}
	f.Add("a‍́ b", uint16(3))
	f.Add("=≮", uint16(5))
	f.Add("\r\n\r\n x   y", uint16(50))

	f.Fuzz(func(t *testing.T, in string, m uint16) {
		max := int(m)
		out := Clean(in, max)
		if !utf8.ValidString(out) {
			t.Fatalf("invalid UTF-8: %q", out)
		}
		if n := utf8.RuneCountInString(out); n > max {
			t.Fatalf("%d runes > max %d", n, max)
		}
		for _, r := range out {
			if forbidden(r) {
				t.Fatalf("forbidden rune %U in %q", r, out)
			}
		}
		if again := Clean(out, max); again != out {
			t.Fatalf("not idempotent: %q -> %q", out, again)
		}
		if out != strings.TrimSpace(out) || strings.Contains(out, "  ") || strings.Contains(out, "\n\n\n") {
			t.Fatalf("whitespace not normalized: %q", out)
		}

		one := OneLine(in, max)
		if !utf8.ValidString(one) || utf8.RuneCountInString(one) > max || strings.ContainsAny(one, "\n<>") {
			t.Fatalf("bad OneLine output %q", one)
		}
		if again := OneLine(one, max); again != one {
			t.Fatalf("OneLine not idempotent: %q -> %q", one, again)
		}
	})
}
