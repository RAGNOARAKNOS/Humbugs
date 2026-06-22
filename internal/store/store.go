// Package store persists coins and their stock snapshots in a SQLite database.
package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ragnoaraknos/Humbugs/internal/model"
	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo)
)

const schema = `
CREATE TABLE IF NOT EXISTS coins (
  sku        TEXT PRIMARY KEY,
  name       TEXT,
  url        TEXT,
  first_seen TIMESTAMP,
  last_seen  TIMESTAMP
);
CREATE TABLE IF NOT EXISTS snapshots (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  sku          TEXT NOT NULL,
  captured_at  TIMESTAMP NOT NULL,
  price        REAL,
  limited_edition_presentation INTEGER,
  total_available     INTEGER,
  backorder_qty       INTEGER,
  preorder_qty        INTEGER,
  purchase_qty        INTEGER,
  status_message      TEXT,
  shipping_message    TEXT,
  display_price       TEXT,
  raw_json     TEXT
);
CREATE INDEX IF NOT EXISTS idx_snap_sku_time ON snapshots(sku, captured_at);
`

// migrations are idempotent column additions applied to databases created by an
// earlier schema. SQLite has no "ADD COLUMN IF NOT EXISTS", so duplicate-column
// errors are tolerated.
var migrations = []string{
	`ALTER TABLE snapshots ADD COLUMN status_message TEXT`,
	`ALTER TABLE snapshots ADD COLUMN shipping_message TEXT`,
	`ALTER TABLE snapshots ADD COLUMN display_price TEXT`,
}

// Store wraps the SQLite database handle.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies the
// schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db %q: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate %q: %w", m, err)
		}
	}
	return &Store{db: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

// UpsertCoin records (or refreshes) a tracked coin's identity.
func (s *Store) UpsertCoin(sku, name, url string, at time.Time) error {
	_, err := s.db.Exec(`
INSERT INTO coins (sku, name, url, first_seen, last_seen)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(sku) DO UPDATE SET name=excluded.name, url=excluded.url, last_seen=excluded.last_seen`,
		sku, name, url, at, at)
	if err != nil {
		return fmt.Errorf("upsert coin %q: %w", sku, err)
	}
	return nil
}

// InsertSnapshot appends a stock snapshot row.
func (s *Store) InsertSnapshot(snap model.Snapshot) error {
	_, err := s.db.Exec(`
INSERT INTO snapshots
  (sku, captured_at, price, limited_edition_presentation, total_available,
   backorder_qty, preorder_qty, purchase_qty, status_message, shipping_message,
   display_price, raw_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.SKU, snap.CapturedAt, snap.Price, snap.LimitedEditionPresentation,
		snap.TotalAvailable, snap.BackorderAvailableQuantity,
		snap.PreorderAvailableQuantity, snap.PurchaseAvailableQuantity,
		snap.StatusMessage, snap.ShippingMessage, snap.DisplayPrice, snap.RawJSON)
	if err != nil {
		return fmt.Errorf("insert snapshot for %q: %w", snap.SKU, err)
	}
	return nil
}

// Coins returns every tracked coin with its most recent snapshot attached.
func (s *Store) Coins() ([]model.Coin, error) {
	rows, err := s.db.Query(`SELECT sku, name, url, first_seen, last_seen FROM coins ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query coins: %w", err)
	}
	defer rows.Close()

	var coins []model.Coin
	for rows.Next() {
		var c model.Coin
		if err := rows.Scan(&c.SKU, &c.Name, &c.URL, &c.FirstSeen, &c.LastSeen); err != nil {
			return nil, fmt.Errorf("scan coin: %w", err)
		}
		coins = append(coins, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range coins {
		latest, prev, err := s.lastTwo(coins[i].SKU)
		if err != nil {
			return nil, err
		}
		coins[i].Latest = latest
		coins[i].Previous = prev
	}
	return coins, nil
}

// lastTwo returns the most recent and second-most-recent snapshots for a SKU,
// either of which may be nil if too few snapshots exist.
func (s *Store) lastTwo(sku string) (latest, prev *model.Snapshot, err error) {
	rows, err := s.db.Query(`
SELECT sku, captured_at, price, limited_edition_presentation, total_available,
       backorder_qty, preorder_qty, purchase_qty,
       COALESCE(status_message,''), COALESCE(shipping_message,''),
       COALESCE(display_price,''), raw_json
FROM snapshots WHERE sku = ? ORDER BY captured_at DESC LIMIT 2`, sku)
	if err != nil {
		return nil, nil, fmt.Errorf("last two snapshots for %q: %w", sku, err)
	}
	defer rows.Close()

	var snaps []model.Snapshot
	for rows.Next() {
		snap, err := scanSnapshot(rows)
		if err != nil {
			return nil, nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snaps = append(snaps, *snap)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(snaps) > 0 {
		latest = &snaps[0]
	}
	if len(snaps) > 1 {
		prev = &snaps[1]
	}
	return latest, prev, nil
}

// History returns all snapshots for a SKU in chronological order.
func (s *Store) History(sku string) ([]model.Snapshot, error) {
	rows, err := s.db.Query(`
SELECT sku, captured_at, price, limited_edition_presentation, total_available,
       backorder_qty, preorder_qty, purchase_qty,
       COALESCE(status_message,''), COALESCE(shipping_message,''),
       COALESCE(display_price,''), raw_json
FROM snapshots WHERE sku = ? ORDER BY captured_at ASC`, sku)
	if err != nil {
		return nil, fmt.Errorf("query history for %q: %w", sku, err)
	}
	defer rows.Close()

	var snaps []model.Snapshot
	for rows.Next() {
		snap, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		snaps = append(snaps, *snap)
	}
	return snaps, rows.Err()
}

// scanner abstracts over *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(sc scanner) (*model.Snapshot, error) {
	var snap model.Snapshot
	err := sc.Scan(&snap.SKU, &snap.CapturedAt, &snap.Price,
		&snap.LimitedEditionPresentation, &snap.TotalAvailable,
		&snap.BackorderAvailableQuantity, &snap.PreorderAvailableQuantity,
		&snap.PurchaseAvailableQuantity, &snap.StatusMessage, &snap.ShippingMessage,
		&snap.DisplayPrice, &snap.RawJSON)
	if err != nil {
		return nil, err
	}
	return &snap, nil
}
