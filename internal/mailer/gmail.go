// Package mailer sends the report through Gmail's SMTP server using an
// App Password (Google Account → Security → 2-Step Verification → App passwords).
package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/smtp"
	"strings"
	"time"
)

const (
	gmailHost = "smtp.gmail.com"
	gmailAddr = "smtp.gmail.com:587" // net/smtp upgrades to TLS via STARTTLS
)

type Gmail struct {
	User        string // your full Gmail address
	AppPassword string // 16-char app password, NOT your normal password
}

func (g Gmail) Send(to []string, subject, text, html string) error {
	msg, err := BuildMessage(g.User, to, subject, text, html, time.Now())
	if err != nil {
		return err
	}
	auth := smtp.PlainAuth("", g.User, g.AppPassword, gmailHost)
	if err := smtp.SendMail(gmailAddr, auth, g.User, to, msg); err != nil {
		return fmt.Errorf("gmail send: %w", err)
	}
	return nil
}

// BuildMessage creates a multipart/alternative MIME message so mail clients
// show the HTML version and fall back to plain text.
func BuildMessage(from string, to []string, subject, text, html string, now time.Time) ([]byte, error) {
	boundaryBytes := make([]byte, 12)
	if _, err := rand.Read(boundaryBytes); err != nil {
		return nil, err
	}
	boundary := "mb-" + hex.EncodeToString(boundaryBytes)

	var b bytes.Buffer
	hdr := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	hdr("From", from)
	hdr("To", strings.Join(to, ", "))
	hdr("Subject", mime.QEncoding.Encode("utf-8", subject)) // subject has 中文 / °
	hdr("Date", now.Format(time.RFC1123Z))
	hdr("MIME-Version", "1.0")
	hdr("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")

	for _, part := range []struct{ ctype, body string }{
		{"text/plain", text},
		{"text/html", html},
	} {
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		hdr("Content-Type", part.ctype+"; charset=utf-8")
		hdr("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		qp := quotedprintable.NewWriter(&b)
		if _, err := qp.Write([]byte(part.body)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes(), nil
}
