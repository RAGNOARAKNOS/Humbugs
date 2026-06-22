# Humbugs

A set of useful tools to track the products sold via the UK Royal Mint.

Humbugs watches a list of Royal Mint coin pages, records their stock levels over
time into a local SQLite database, and serves a small web dashboard so you can
see current availability and history at a glance. It automates the page-scraping
the original [bookmarklet](docs/bookmarklet.md) did by hand. See the
[Concept of Operations](docs/conops.md) for the why, and the
[roadmap](docs/roadmap.md) for planned additions.

## Install / build

Requires Go 1.24+ (pure-Go dependencies — no C toolchain needed).

```sh
go build -o humbugs ./cmd/humbugs
```

## Dependencies

Humbugs deliberately keeps its dependency footprint small.

**Backend (Go modules — see [`go.mod`](go.mod) for exact versions):**

- [`github.com/PuerkitoBio/goquery`](https://github.com/PuerkitoBio/goquery) —
  jQuery-style HTML parsing used to scrape Royal Mint product pages.
- [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) — pure-Go SQLite
  driver for the history database (no C toolchain / CGO required).
- [`gopkg.in/yaml.v3`](https://github.com/go-yaml/yaml) — parses `coins.yaml`.

Everything else in `go.mod` is an indirect (transitive) dependency pulled in by
the above.

**Frontend (loaded at runtime via CDN):**

- [Chart.js 4](https://www.chartjs.org/) — renders the stock-history chart on
  each coin page. Loaded from jsDelivr in
  [`internal/web/templates/coin.html`](internal/web/templates/coin.html); a
  network connection is needed to view that chart.

## Configure

Copy the example config and add the coin product pages you want to track:

```sh
cp coins.example.yaml coins.yaml
```

```yaml
coins:
  - name: 2024 Gold Proof Sovereign   # optional label; page name used if omitted
    url: https://www.royalmint.com/...your-coin...
```

## Use

```sh
humbugs scrape   # one pass: fetch each coin, append a snapshot to humbugs.db
humbugs list     # print tracked coins + latest stock to the terminal
humbugs serve    # dashboard at http://localhost:8080
```

Common flags: `--config coins.yaml`, `--db humbugs.db`, and (serve)
`--port 8080` / `--addr :8080`.

## Choosing a port

By default the dashboard listens on port `8080`. To run it elsewhere, use any of
(in order of precedence):

```sh
humbugs serve --port 9090            # shorthand, listens on :9090
humbugs serve --addr 127.0.0.1:9090  # full host:port (e.g. bind to localhost only)
HUMBUGS_ADDR=:9090 humbugs serve     # environment variable (handy for services)
```

`--port` overrides `--addr`, which overrides `$HUMBUGS_ADDR`, which overrides the
`:8080` default. On Windows PowerShell, set the env var with
`$env:HUMBUGS_ADDR = ':9090'` before running.

## Scheduling (Windows Task Scheduler, every 3 hours)

`humbugs scrape` does a single pass and exits, so history is built by running it
on a schedule. Below, Humbugs is assumed to live in `C:\Humbugs\` containing
`humbugs.exe` and `coins.yaml`; adjust paths to suit. **Always pass absolute
paths** for `--config` and `--db`, because a scheduled task runs from
`C:\Windows\System32`, not your folder.

### Option A — command line (fastest)

Open **PowerShell** (or Command Prompt) and create the task with `schtasks`:

```
schtasks /Create /TN "Humbugs Scrape" /SC HOURLY /MO 3 /RL LIMITED /F ^
  /TR "\"C:\Humbugs\humbugs.exe\" scrape --config \"C:\Humbugs\coins.yaml\" --db \"C:\Humbugs\humbugs.db\""
```

- `/SC HOURLY /MO 3` runs it **every 3 hours**.
- `/F` overwrites an existing task of the same name (handy when re-running).
- Run it now to test: `schtasks /Run /TN "Humbugs Scrape"`
- Inspect / delete: `schtasks /Query /TN "Humbugs Scrape" /V /FO LIST` ·
  `schtasks /Delete /TN "Humbugs Scrape" /F`

To also keep a log of each run, point the task at `cmd` and redirect output:

```
/TR "cmd /c \"\"C:\Humbugs\humbugs.exe\" scrape --config \"C:\Humbugs\coins.yaml\" --db \"C:\Humbugs\humbugs.db\" >> \"C:\Humbugs\scrape.log\" 2>&1\""
```

### Option B — Task Scheduler GUI

1. Open **Task Scheduler** → **Create Task…** (not "Basic Task").
2. **General**: name it `Humbugs Scrape`. Choose "Run only when user is logged
   on" (simplest) or "Run whether user is logged on or not" (needs your
   password; survives logoff).
3. **Triggers** → **New**: Begin "On a schedule", Daily. Under **Advanced
   settings**, tick **Repeat task every** and set it to **3 hours** for a
   duration of **Indefinitely**. Also tick **Run task as soon as possible after
   a scheduled start is missed** so it catches up after sleep/shutdown.
4. **Actions** → **New**: Action "Start a program".
   - **Program/script**: `C:\Humbugs\humbugs.exe`
   - **Add arguments**: `scrape --config "C:\Humbugs\coins.yaml" --db "C:\Humbugs\humbugs.db"`
   - **Start in**: `C:\Humbugs`
5. **Conditions**: for a laptop, untick "Start the task only if the computer is
   on AC power" so it still runs on battery.
6. Click **OK**, then right-click the task → **Run** to test immediately.

### Verify it worked

```
humbugs list --db C:\Humbugs\humbugs.db
```

You should see updated "Last checked" times, and a few hours later the **Trend**
column will start showing movement since the previous run.

Keep the cadence modest (every 3 hours is fine) to be polite to the Mint's
servers; Humbugs already pauses between requests and identifies itself via a
User-Agent.

Run `humbugs serve` whenever you want to view the dashboard (it reads the same
database the scheduled scrapes write to).

## Releasing

Container images are published to GHCR **only for version tags**. Ordinary
pushes and pull requests run the tests but never build or publish an image, so
cutting a release is a deliberate, tag-driven step.

Tags must follow semantic versioning with a leading `v` (e.g. `v1.2.3`) — the
workflow matches `v*` and derives the image tags from it.

```sh
# Make sure main is green and up to date first
git checkout main
git pull

# Create an annotated tag and push it
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
```

Pushing the tag triggers CI to run the tests and then build and publish a
multi-arch (amd64/arm64) image to
`ghcr.io/ragnoaraknos/humbugs` with these tags:

| Git tag  | Image tags published                        |
| -------- | ------------------------------------------- |
| `v1.2.3` | `1.2.3`, `1.2`, `1`, and `latest`           |

Pull a specific release (or just `latest`) with:

```sh
docker pull ghcr.io/ragnoaraknos/humbugs:1.0.0
```

To delete a tag pushed by mistake (before relying on it):

```sh
git push origin :refs/tags/v1.0.0   # delete remote tag
git tag -d v1.0.0                   # delete local tag
```
