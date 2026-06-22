# Roadmap / Future Additions

Ideas for extending Humbugs beyond its current capability (configured coin list,
scheduled one-shot scrapes into SQLite, web dashboard with availability badges,
scarcity bars, price, and time-normalised trend analysis). Unordered; pick by
value when picking up the project again.

## Alerts / notify mode
A `humbugs scrape --notify` flag that, after a scrape pass, prints a digest of
only the coins whose state changed in a way worth acting on, for example:
- became purchasable (availability `available`),
- crossed a scarcity threshold (e.g. dropped below 10% of the edition remaining),
- changed availability status (e.g. `preorder → backorder`, or sold out).

Designed to be piped from a scheduled task into a desktop/email/webhook
notification. The building blocks already exist: `Coin.Change()` exposes
availability transitions and `Snapshot.RemainingPercent()` the scarcity level.

## Price on the history chart
The per-coin history chart currently plots quantities only. The numeric `price`
is already stored per snapshot, so it could be added as a second axis to spot
price changes over time alongside stock movement.

## Longer-window trend analysis
The current trend compares only the latest two snapshots ("since last check").
A longer window (e.g. average rate over the last 7 days, or since first seen)
would smooth out noise and give a steadier sell-through estimate. The full
history is available via `Store.History(sku)`.

## Dashboard quality-of-life
- Auto-refresh / "last updated" indicator on the dashboard.
- Client-side sorting and filtering (e.g. show only buyable, or only scarce).
- A simple search box when the tracked list grows large.

## Resilience to site changes
The most likely maintenance trigger is a change to the Royal Mint page
structure. Each snapshot already stores the full raw `data-product-settings`
JSON (`raw_json`), so a future migration/reprocessing tool could re-derive
modelled fields from history without re-scraping.

## Capture more fields
The settings JSON carries more than Humbugs currently models (e.g. media URLs,
category, brand, ratings). Worth modelling selectively if they prove useful for
buying decisions — again, `raw_json` means nothing is lost in the meantime.

## Scheduled scraping in containers
The Docker image runs `serve` as a long-lived process, but `scrape` is a
one-shot command, so containerised deployments need something external to
refresh the data on a schedule. Options worth offering:

- **Cron sidecar** — add an [ofelia](https://github.com/mcuadros/ofelia) (or
  plain cron) service to `docker-compose.yml` that runs
  `humbugs scrape` against the shared `/data` volume every few hours. Keeps the
  scraper decoupled from the web server and easy to retune.
- **Built-in scheduler** — alternatively, a `humbugs serve --scrape-every 3h`
  flag could run periodic scrapes inside the serve process, removing the need
  for a sidecar at the cost of coupling the two concerns.

Either way the polite `politeDelay` between fetches should be preserved.

## Managing removed / stale coins
Removing a coin from `coins.yaml` currently just stops it being scraped — its
`coins` row and history remain, and it keeps showing on the dashboard and in
`humbugs list` with a "Last checked" time that no longer advances. This retains
history (often desirable) but offers no explicit control. Two complementary
additions:

- **`humbugs remove <sku>`** — a command to permanently delete a coin and its
  snapshots from the database (the manual equivalent today is two `DELETE`
  statements against the `coins` and `snapshots` tables).
- **`humbugs list --stale`** (and/or a dashboard indicator) — flag coins not
  seen in the last N hours, so entries dropped from the config or pages that
  stopped resolving are easy to spot. `Coin.LastSeen` already provides the
  signal.

## Collective multi-contributor deployment

Today Humbugs is a single-process tool: one machine scrapes into a local SQLite
file and serves the dashboard from it. An alternative deployment lets *several
users contribute scrapes into one shared database* — distributing the scraping
load while centralising the history everyone views.

The model: a central server owns the database and the coin list and exposes a
token-gated ingest API; contributors run a lightweight client that fetches the
coin list, scrapes locally (preserving `politeDelay`), and POSTs results back.
Reads stay public; writes require a token.

Work involved, roughly in order:

- **Concurrency-safe database.** SQLite is single-writer, so many concurrent
  contributors need Postgres. `internal/store` already uses `database/sql`, so
  the change is mostly SQL dialect (`$1` placeholders, `BIGSERIAL`, `TIMESTAMPTZ`,
  `ON CONFLICT`), a DSN-based `Open`, and replacing the ad-hoc `ALTER TABLE`
  migrations with a versioned schema.
- **Provenance & dedup.** Add a `source` column to `snapshots` so each point is
  attributed to a contributor, and decide a dedup policy when two people scrape
  the same coin (e.g. accept all and dedupe per time-bucket in `History`).
- **Ingest API.** A `POST /api/ingest` endpoint in `internal/web`, sharing the
  `snapshotFrom()` mapping with the client so both build identical models.
- **Token auth.** A `tokens` table plus middleware on the ingest route
  (`Authorization: Bearer <token>`, 401 otherwise) and a `humbugs token
  add/list/revoke` admin command. Read routes remain unauthenticated.
- **Central coin list.** Promote the `coins` table to the source of truth with a
  `GET /api/coins` endpoint and a `humbugs coin add <url>` command, replacing the
  per-contributor `coins.yaml`.
- **Contributor client.** A `humbugs contribute --server <url> --token <t>`
  command that fetches the coin list, scrapes, and pushes snapshots. The local
  `scrape` command stays for admin/dev use.
- **Deployment.** Add a Postgres service to `docker-compose.yml` and read
  `DATABASE_URL` from the environment; a hosted setup pairs managed Postgres
  (Neon/Supabase/RDS) with a container host (Fly.io/Render) terminating HTTPS.

This is a larger change than the other roadmap items and effectively makes
Humbugs a small service rather than a CLI tool — worth doing only if shared,
crowd-sourced stock history is a goal.
