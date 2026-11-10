// Command morningbrief fetches today's forecast, stores it, compares it
// with earlier days, and emails a "what to wear" report.
//
// Run it once each morning from cron. See README.md for setup.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed timezone data so Asia/Taipei works on minimal systems

	"morningbrief/internal/advice"
	"morningbrief/internal/cwa"
	"morningbrief/internal/mailer"
	"morningbrief/internal/report"
	"morningbrief/internal/store"
	"morningbrief/internal/weather"
)

// place is one district we report on.
type place struct {
	city, district, dataset string
}

func (p place) label() string { return p.city + p.district }

type config struct {
	cwaKey        string
	places        []place
	dataDir       string
	retentionDays int
	gmailUser     string
	gmailPass     string
	mailTo        []string
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// parsePlaces turns "內湖區,大安區" or "內湖區,新竹市/東區" into places.
// Entries without a "city/" prefix use defaultCity.
func parsePlaces(defaultCity, list string) ([]place, error) {
	var out []place
	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		city, district := defaultCity, entry
		if c, d, ok := strings.Cut(entry, "/"); ok {
			city, district = c, d
		}
		p := place{city: cwa.NormalizeName(city), district: cwa.NormalizeName(district)}
		id, err := cwa.DatasetForCity(p.city)
		if err != nil {
			return nil, err
		}
		p.dataset = id
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no districts configured")
	}
	return out, nil
}

func loadConfig(districtFlag string) (config, error) {
	c := config{
		cwaKey:    env("CWA_API_KEY", ""),
		dataDir:   env("DATA_DIR", "./data"),
		gmailUser: env("GMAIL_USER", ""),
		gmailPass: strings.ReplaceAll(env("GMAIL_APP_PASSWORD", ""), " ", ""),
	}
	districts := env("DISTRICTS", "內湖區") // 葫洲里 is in 內湖區
	if districtFlag != "" {
		districts = districtFlag
	}
	var err error
	if c.places, err = parsePlaces(env("CITY", "臺北市"), districts); err != nil {
		return c, err
	}
	days, err := strconv.Atoi(env("RETENTION_DAYS", "30"))
	if err != nil || days < 1 {
		return c, fmt.Errorf("RETENTION_DAYS must be a positive integer")
	}
	c.retentionDays = days
	for _, addr := range strings.Split(env("MAIL_TO", c.gmailUser), ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			c.mailTo = append(c.mailTo, addr)
		}
	}
	return c, nil
}

func main() {
	district := flag.String("district", "", `override DISTRICTS for this run, e.g. 大安區 or "內湖區,大安區"`)
	dryRun := flag.Bool("dry-run", false, "print the email instead of sending it")
	fixture := flag.String("fixture", "", "read the CWA response from this file instead of calling the API")
	dateFlag := flag.String("date", "", "pretend today is YYYY-MM-DD (for testing)")
	htmlOut := flag.String("html-out", "", "with -dry-run, also write the HTML email to this file")
	flag.Parse()

	if err := run(*district, *dryRun, *fixture, *dateFlag, *htmlOut); err != nil {
		log.Fatal(err) // prints the error and exits with status 1, so cron can notice
	}
}

func run(districtFlag string, dryRun bool, fixture, dateFlag, htmlOut string) error {
	cfg, err := loadConfig(districtFlag)
	if err != nil {
		return err
	}
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return err
	}
	today := time.Now().In(tz)
	if dateFlag != "" {
		if today, err = time.ParseInLocation("2006-01-02", dateFlag, tz); err != nil {
			return fmt.Errorf("-date: %w", err)
		}
	}
	if fixture == "" && cfg.cwaKey == "" {
		return fmt.Errorf("CWA_API_KEY is not set")
	}

	var sections []report.Section
	var headlines []string
	for _, p := range cfg.places {
		sec, headline, err := placeSection(cfg, p, today, fixture)
		if err != nil {
			return fmt.Errorf("%s: %w", p.label(), err)
		}
		sections = append(sections, sec)
		headlines = append(headlines, p.district+" "+headline)
	}

	subject := fmt.Sprintf("Morning brief %s · %s", today.Format("01/02 Mon"), headlines[0])
	email, err := report.Compose(subject, today, sections)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Println("Subject:", email.Subject)
		fmt.Println()
		fmt.Print(email.Text)
		if htmlOut != "" {
			return os.WriteFile(htmlOut, []byte(email.HTML), 0o644)
		}
		return nil
	}
	if cfg.gmailUser == "" || cfg.gmailPass == "" {
		return fmt.Errorf("GMAIL_USER and GMAIL_APP_PASSWORD must be set (or use -dry-run)")
	}
	g := mailer.Gmail{User: cfg.gmailUser, AppPassword: cfg.gmailPass}
	if err := g.Send(cfg.mailTo, email.Subject, email.Text, email.HTML); err != nil {
		return err
	}
	log.Printf("sent to %s", strings.Join(cfg.mailTo, ", "))
	return nil
}

// placeSection fetches, stores and renders one district.
func placeSection(cfg config, p place, today time.Time, fixture string) (report.Section, string, error) {
	var loc *cwa.Location
	var raw []byte
	var err error
	if fixture != "" {
		if raw, err = os.ReadFile(fixture); err != nil {
			return report.Section{}, "", err
		}
		loc, err = cwa.Parse(raw, p.district)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		loc, raw, err = cwa.NewClient(cfg.cwaKey).TownshipForecast(ctx, p.dataset, p.district)
	}
	if err != nil {
		return report.Section{}, "", err
	}

	sum := weather.Summarize(loc, today, p.label())
	if len(sum.Periods) == 0 {
		return report.Section{}, "", fmt.Errorf("forecast has no data for %s (is the date right?)", sum.Date)
	}

	// Each district keeps its own history, e.g. data/臺北市內湖區/days/2026-11-04.json
	st, err := store.New(filepath.Join(cfg.dataDir, p.label()))
	if err != nil {
		return report.Section{}, "", err
	}
	if n, err := st.Prune(today, cfg.retentionDays); err != nil {
		log.Printf("%s prune: %v", p.label(), err) // not fatal: still send today's email
	} else if n > 0 {
		log.Printf("%s: pruned %d old file(s)", p.label(), n)
	}
	history, err := st.History(sum.Date, cfg.retentionDays)
	if err != nil {
		return report.Section{}, "", err
	}
	if err := st.Save(sum, raw); err != nil {
		return report.Section{}, "", err
	}

	adv := advice.Build(sum, history)
	sec, err := report.WeatherSection(sum, history, adv)
	sec.Title = p.label()
	return sec, adv.Headline, err
}
