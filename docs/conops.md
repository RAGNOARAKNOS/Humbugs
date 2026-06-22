# Concept of Operations (CONOPS)

## Statement of the goals and objectives of the system

As a collector of coins, from the UK Royal Mint, I am very keen to know the current stock levels and availabilities of certain coins.
I have previously written a [bookmarklet](bookmarklet.md) that when initialised on a page on the Mint website is able to extract, price, stock levels and production quantities.
I am now looking to expand this tool, such that it can regularly check a list of known coins, produce a nice visual indicator of availability, and importantly, keep a history of these stats, such that I can inform my buying decisions with data.

## Strategies, tactics, policies, and constraints affecting the system

The data is obtained the same way the bookmarklet obtains it: each Royal Mint
product page embeds a `data-product-settings` JSON attribute carrying the
product name, SKU, price, and stock summary. Humbugs fetches the page and reads
that attribute server-side rather than driving a browser.

Constraints and policies:

- **Single-user and local.** Humbugs runs on the collector's own machine; the
  history is kept in a local SQLite file and never published.
- **Be a good citizen.** Requests are made sequentially with a short delay
  between them, at a modest frequency (a few times a day), using a User-Agent
  that identifies the tool. The aim is data for personal buying decisions, not
  load on the Mint's site.
- **Resilient to change.** The full raw settings JSON is stored alongside the
  modelled fields, so a future change to the page structure does not silently
  lose data and can be reprocessed.
- **Minimal footprint.** Pure-Go dependencies only, so the tool builds and runs
  on Windows without a C toolchain.

## Organisations, activities, and interactions among participants and stakeholders

- **Collector (user).** The sole stakeholder: chooses which coins to track, runs
  the tool, and reads the dashboard to inform purchases.
- **The UK Royal Mint website.** The external data source. Humbugs is a read-only
  consumer of its public product pages and has no other relationship with it.

## Clear statement of responsibilities and authorities delegated

- The collector is responsible for the list of tracked coins (`coins.yaml`), for
  scheduling scrapes, and for any buying decisions made from the data.
- Humbugs is responsible only for faithfully capturing, storing, and presenting
  the stock figures it reads; it does not buy, alert externally, or act on the
  user's behalf.

## Specific operational processes for fielding the system

1. Build the binary and create `coins.yaml` from `coins.example.yaml`.
2. Run `humbugs scrape` once to confirm coins resolve and snapshots are stored
   (verify with `humbugs list`).
3. Register a scheduled task (Windows Task Scheduler / cron) to run
   `humbugs scrape` periodically, building history over time.
4. Run `humbugs serve` on demand to view current availability and history charts.

## Processes for initiating, developing, maintaining, and retiring the system

- **Initiating / developing.** Greenfield Go project; see the README for build
  and usage. Functionality is split into small packages (config, scraper, store,
  web) to keep each piece testable.
- **Maintaining.** The most likely maintenance trigger is a change to the Royal
  Mint page structure; the stored raw JSON aids diagnosis. Schema changes are
  applied idempotently at startup (`CREATE TABLE IF NOT EXISTS`).
- **Retiring.** The system is self-contained: removing the scheduled task stops
  collection, and deleting the binary and the SQLite database removes it
  entirely. The historical data file can be archived beforehand if desired.