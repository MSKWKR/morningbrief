# morningbrief (draft)

Every morning: fetch the CWA (中央氣象署) forecast, keep 30 days of history,
compare today with earlier days, and email a "what to wear" brief via Gmail.

## Setup

1. **CWA API key**: register at https://opendata.cwa.gov.tw → 會員 → 取得授權碼.
2. **Gmail App Password**: Google Account → Security → 2-Step Verification → App passwords.
3. **Anthropic API key** (for tech news, optional): create a dedicated key in its
   own workspace at https://console.anthropic.com and set a monthly spend limit there.
4. Copy `.env.example` to `.env`, fill it in, and `chmod 600 .env`.

## Config (env vars)

| var | default | notes |
|---|---|---|
| `CWA_API_KEY` | – | required |
| `CITY` | `臺北市` | 台/臺 both fine |
| `DISTRICTS` | `內湖區` | comma list, e.g. `內湖區,大安區`; other city: `新竹市/東區` |
| `DATA_DIR` | `./data` | one folder per district |
| `RETENTION_DAYS` | `30` | older files are deleted each run |
| `GMAIL_USER` / `GMAIL_APP_PASSWORD` | – | required to send |
| `MAIL_TO` | `GMAIL_USER` | comma list |
| `ANTHROPIC_API_KEY` | – | optional; without it news is skipped with a note (exit 1 after sending) |
| `NEWS_MODEL` | `claude-haiku-5-5` | model used to summarize each article |

CWA forecasts go down to district level only, so 葫洲里 → 內湖區.

## Run

```sh
go run ./cmd/morningbrief -dry-run                       # print, don't send
go run ./cmd/morningbrief -dry-run -district 大安區       # one-off district
go run ./cmd/morningbrief -dry-run -district 內湖區 \
   -fixture testdata/F-D0047-061-2026-11-04.json -date 2026-11-04   # offline (fake data)
go run ./cmd/morningbrief -dry-run -no-news              # weather only
go run ./cmd/morningbrief -dry-run -news-fixture testdata/news-fixture.json   # fake news, no network or API key
go test ./...
```

## Miscellaneous (Hacker News) & safety

The email gets a "Miscellaneous" section: up to 5 Hacker News stories, each
summarized in 2-3 sentences. Stories are chosen from the top 60: the
best-scoring ones submitted in the last 24 hours that do not look like a
resubmitted old article (a "(2013)" tag in the title, or an old date in the
URL). There is no minimum score, so a slow day still gives a full section.
Because the job runs every 24 hours, a story is never included twice, and
nothing is stored to track that. Fetched web pages are untrusted, so:

- Stories are fetched sequentially with the User-Agent
  `morningbrief/0.1 (personal news digest)`, honouring robots.txt.
- SSRF guard: private, loopback and link-local IPs are blocked at dial time,
  only ports 80/443 are allowed, and proxies are not used.
- `internal/sanitize` is the only path from fetched text to the model.
- The model has no tools and gets one call per article.
- Links in the email come only from HN data, never from the model.
- Failure tiers: one failing story shows its title and link only; if news as a
  whole fails, the email says "Miscellaneous news unavailable today (reason)". The weather
  email is still sent, then the process exits 1 so cron notices. Weather
  failures still fail fast, before any news work.
- The whole news step has a 15-minute deadline.

## Deploy on TrueNAS

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o morningbrief ./cmd/morningbrief
```
Copy `morningbrief`, `run.sh` and `.env` to a dataset, `chmod +x` both,
then add a cron job in TrueNAS (System → Advanced → Cron Jobs), e.g.
`30 6 * * *` → `/mnt/pool/apps/morningbrief/run.sh`.
CWA updates the forecast at 05:30, so run after that. TrueNAS cron runs in
the NAS's own timezone, so check the time if the NAS isn't set to Asia/Taipei.

The Docker image runs as UID/GID `10001`, not root. Make the mounted data
directory writable by that user, e.g.
`chown -R 10001:10001 /mnt/pool/apps/morningbrief/data`.

## Storage layout

```
data/臺北市內湖區/days/2026-11-04.json   summary (what comparisons use)
data/臺北市內湖區/raw/2026-11-04.json    raw CWA response
```

## Layout / next steps

- `internal/cwa` API client + city→dataset map
- `internal/weather` raw forecast → DaySummary
- `internal/store` per-day JSON + retention
- `internal/advice` clothing rules + comparison
- `internal/report` email sections (weather and tech news)
- `internal/mailer` Gmail SMTP
- `internal/hn` Hacker News top-stories client
- `internal/fetch` SSRF-hardened article fetcher + readable-text extraction
- `internal/sanitize` cleans fetched text before it reaches the model
- `internal/summarize` Claude API client, one call per article
- `internal/news` pipeline tying the above together (+ fixture loader)
