# morningbrief (draft)

Every morning: fetch the CWA (中央氣象署) forecast, keep 30 days of history,
compare today with earlier days, and email a "what to wear" brief via Gmail.

## Setup

1. **CWA API key**: register at https://opendata.cwa.gov.tw → 會員 → 取得授權碼.
2. **Gmail App Password**: Google Account → Security → 2-Step Verification → App passwords.
3. Copy `.env.example` to `.env` and fill it in.

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

CWA forecasts go down to district level only, so 葫洲里 → 內湖區.

## Run

```sh
go run ./cmd/morningbrief -dry-run                       # print, don't send
go run ./cmd/morningbrief -dry-run -district 大安區       # one-off district
go run ./cmd/morningbrief -dry-run -district 內湖區 \
   -fixture testdata/F-D0047-061-2026-11-04.json -date 2026-11-04   # offline (fake data)
go test ./...
```

## Deploy on TrueNAS

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o morningbrief ./cmd/morningbrief
```
Copy `morningbrief`, `run.sh` and `.env` to a dataset, `chmod +x` both,
then add a cron job in TrueNAS (System → Advanced → Cron Jobs), e.g.
`30 6 * * *` → `/mnt/pool/apps/morningbrief/run.sh`.
CWA updates the forecast at 05:30, so run after that. TrueNAS cron runs in
the NAS's own timezone, so check the time if the NAS isn't set to Asia/Taipei.

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
- `internal/report` email sections (add a tech-news Section here later)
- `internal/mailer` Gmail SMTP

Next: tech news summary via Claude Sonnet as a second `report.Section`.
