package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// Headline is one feed item. Every field comes from a third-party feed, so it
// is only ever rendered through html/template, and URL must already have been
// validated as http(s) by the caller.
type Headline struct {
	Title   string
	URL     string
	Source  string // publication name, e.g. "Krebs on Security"
	Summary string // plain text
}

// KEVEntry is one CISA Known Exploited Vulnerabilities record. URL is built
// by the caller from a validated CVE ID.
type KEVEntry struct {
	CVE         string
	URL         string
	Vendor      string
	Product     string
	Name        string
	Description string
	DateAdded   string
	Ransomware  bool // CISA marks known ransomware campaign use
}

// KEVBlock is the vulnerability list. Note is shown instead of the list when
// there are no entries (for example "no new entries today").
type KEVBlock struct {
	Entries []KEVEntry
	Note    string
}

// BriefData describes one themed section. A nil KEV block is left out.
type BriefData struct {
	Title         string
	Sources       []string
	KEV           *KEVBlock
	Headlines     []Headline
	HeadlinesNote string // shown when Headlines is empty
}

// BriefSection renders a themed section (Cybersecurity, Markets, ...).
func BriefSection(d BriefData) (Section, error) {
	var tb strings.Builder
	if d.KEV != nil {
		tb.WriteString("New actively exploited vulnerabilities (CISA KEV)\n")
		if len(d.KEV.Entries) == 0 {
			fmt.Fprintf(&tb, "  %s\n", d.KEV.Note)
		}
		for _, e := range d.KEV.Entries {
			flag := ""
			if e.Ransomware {
				flag = " [known ransomware use]"
			}
			fmt.Fprintf(&tb, "  - %s · %s %s%s\n    %s\n", e.CVE, e.Vendor, e.Product, flag, e.Name)
			if e.Description != "" {
				fmt.Fprintf(&tb, "    %s\n", e.Description)
			}
			fmt.Fprintf(&tb, "    %s\n", e.URL)
		}
		tb.WriteString("\n")
	}
	if len(d.Headlines) > 0 {
		tb.WriteString("Headlines\n")
		for i, h := range d.Headlines {
			fmt.Fprintf(&tb, "  %d. %s (%s)\n     %s\n", i+1, h.Title, h.Source, h.URL)
			if h.Summary != "" {
				fmt.Fprintf(&tb, "     %s\n", h.Summary)
			}
		}
	} else if d.HeadlinesNote != "" {
		fmt.Fprintf(&tb, "Headlines\n  %s\n", d.HeadlinesNote)
	}

	// html/template escapes per context (including href), so there is no
	// string concatenation or template.HTML/URL cast of untrusted data.
	var hb bytes.Buffer
	if err := briefTmpl.Execute(&hb, d); err != nil {
		return Section{}, err
	}
	return Section{Title: d.Title, Text: tb.String(), HTML: template.HTML(hb.String()), Sources: d.Sources}, nil
}

// SectionUnavailable is the one-line note for a section that failed entirely.
func SectionUnavailable(title, reason string) Section {
	msg := title + " unavailable today (" + reason + ")"
	var hb bytes.Buffer
	// The template is static and takes a string, so Execute cannot fail here.
	_ = unavailTmpl.Execute(&hb, msg)
	return Section{Title: title, Text: msg + "\n", HTML: template.HTML(hb.String())}
}

var briefTmpl = template.Must(template.New("b").Parse(`
{{with .KEV}}<h3 style="margin:0 0 6px;font-size:15px">New actively exploited vulnerabilities <span style="font-weight:400;color:#888">(CISA KEV)</span></h3>
{{if .Entries}}<ul style="margin:0 0 14px;padding-left:20px">
{{range .Entries}}<li style="margin:0 0 8px"><a href="{{.URL}}" style="font-weight:600;color:#1a0dab;text-decoration:none">{{.CVE}}</a> · {{.Vendor}} {{.Product}}{{if .Ransomware}} <span style="color:#b3261e;font-size:12px;font-weight:600">known ransomware use</span>{{end}}
<div style="font-size:14px">{{.Name}}</div>{{if .Description}}<div style="font-size:13px;color:#555">{{.Description}}</div>{{end}}</li>
{{end}}</ul>{{else}}<p style="margin:0 0 14px;color:#666">{{.Note}}</p>{{end}}{{end}}
{{if .Headlines}}<h3 style="margin:0 0 6px;font-size:15px">Headlines</h3>
<ol style="margin:0;padding-left:22px">
{{range .Headlines}}<li style="margin:0 0 10px"><a href="{{.URL}}" style="font-weight:600;color:#1a0dab;text-decoration:none">{{.Title}}</a> <span style="color:#888;font-size:12px">{{.Source}}</span>
{{if .Summary}}<p style="margin:3px 0 0;font-size:14px">{{.Summary}}</p>
{{end}}</li>{{end}}
</ol>{{else if .HeadlinesNote}}<h3 style="margin:0 0 6px;font-size:15px">Headlines</h3><p style="margin:0;color:#666">{{.HeadlinesNote}}</p>{{end}}
`))
