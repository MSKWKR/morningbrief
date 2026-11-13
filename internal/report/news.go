package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// newsTitle is the title of the Hacker News section.
const newsTitle = "Miscellaneous"

// NewsItem is one story. Every field may come from untrusted sources (HN
// submitters, LLM output), so it is only ever rendered through html/template.
type NewsItem struct {
	Title         string
	URL           string
	DiscussionURL string
	Points        int
	Comments      int
	Summary       string // empty => title + link only
}

// NewsSection builds the tech-news section.
func NewsSection(items []NewsItem) (Section, error) {
	var tb strings.Builder
	for i, it := range items {
		if i > 0 {
			tb.WriteString("\n")
		}
		fmt.Fprintf(&tb, "%d. %s\n   %s\n", i+1, it.Title, it.URL)
		if it.Summary != "" {
			fmt.Fprintf(&tb, "   %s\n", it.Summary)
		}
		fmt.Fprintf(&tb, "   %d points · %d comments · %s\n", it.Points, it.Comments, it.DiscussionURL)
	}

	// html/template escapes per context (including href), so no string
	// concatenation or template.HTML/URL casts of untrusted data here.
	var hb bytes.Buffer
	if err := newsTmpl.Execute(&hb, items); err != nil {
		return Section{}, err
	}
	return Section{Title: newsTitle, Text: tb.String(), HTML: template.HTML(hb.String()), Sources: []string{"Hacker News"}}, nil
}

// NewsUnavailableSection is the one-line note shown when news failed entirely.
func NewsUnavailableSection(reason string) Section {
	msg := "Miscellaneous news unavailable today (" + reason + ")"
	var hb bytes.Buffer
	// The template is static and takes a string, so Execute cannot fail here.
	_ = unavailTmpl.Execute(&hb, msg)
	return Section{Title: newsTitle, Text: msg + "\n", HTML: template.HTML(hb.String()), Sources: []string{"Hacker News"}}
}

var newsTmpl = template.Must(template.New("n").Parse(`
<ol style="margin:0;padding-left:22px">
{{range .}}<li style="margin:0 0 12px"><a href="{{.URL}}" style="font-weight:600;color:#1a0dab;text-decoration:none">{{.Title}}</a>
{{if .Summary}}<p style="margin:4px 0 0;font-size:14px">{{.Summary}}</p>
{{end}}<p style="margin:4px 0 0;font-size:12px;color:#888">{{if .DiscussionURL}}<a href="{{.DiscussionURL}}" style="color:#888;text-decoration:none">{{.Points}} points · {{.Comments}} comments</a>{{else}}{{.Points}} points · {{.Comments}} comments{{end}}</p>
</li>{{end}}
</ol>
`))

var unavailTmpl = template.Must(template.New("u").Parse(
	`<p style="margin:0;color:#666">{{.}}</p>`))
