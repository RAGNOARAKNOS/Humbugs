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
