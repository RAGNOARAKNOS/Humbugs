// Package model defines the data structures shared across Humbugs: the shape of
// the Royal Mint product settings JSON and the snapshot we persist over time.
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// FlexInt is an integer that tolerates the Royal Mint's loose JSON encoding,
// where a quantity may appear as an int (6252), a float (6252.0), or a quoted
// string ("6252"). It always reads back as a plain int.
type FlexInt int

// UnmarshalJSON parses a number (int or float) or a quoted numeric string.
func (f *FlexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		b = []byte(s)
	}
	v, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	*f = FlexInt(v)
	return nil
}

// StockSummary mirrors the stockSummary object inside a Royal Mint product
// page's data-product-settings attribute.
type StockSummary struct {
	LimitedEditionPresentation FlexInt `json:"LimitedEditionPresentation"`
	TotalAvailable             FlexInt `json:"TotalAvailable"`
	BackorderAvailableQuantity FlexInt `json:"BackorderAvailableQuantity"`
	PreorderAvailableQuantity  FlexInt `json:"PreorderAvailableQuantity"`
	PurchaseAvailableQuantity  FlexInt `json:"PurchaseAvailableQuantity"`
	// StatusMessage and ShippingMessage are free-text context from the page,
	// e.g. "Available to Order" and "Shipping early August".
	StatusMessage   string `json:"StatusMessage"`
	ShippingMessage string `json:"ShippingMessage"`
}

// Product is the subset of the data-product-settings JSON that Humbugs models.
// The full raw JSON is preserved separately so unmodelled fields are not lost.
type Product struct {
	ProductName string `json:"productName"`
	SKU         string `json:"sku"`
	// Price is the plain numeric price as a string (e.g. "25.00"); the Royal
	// Mint encodes it as a JSON string. Use PriceValue for a float.
	Price string `json:"price"`
	// CurrentPrice is the display-ready, currency-formatted price (e.g. "£25.00").
	CurrentPrice string       `json:"currentPrice"`
	StockSummary StockSummary `json:"stockSummary"`
}

// PriceValue parses Price into a float64, returning 0 if it is blank or
// unparseable (e.g. when the page omits or formats it unexpectedly).
func (p Product) PriceValue() float64 {
	v, err := strconv.ParseFloat(p.Price, 64)
	if err != nil {
		return 0
	}
	return v
}

// Snapshot is a single point-in-time capture of a coin's stock, as stored in the
// snapshots table.
type Snapshot struct {
	SKU                        string
	CapturedAt                 time.Time
	Price                      float64
	DisplayPrice               string
	LimitedEditionPresentation int
	TotalAvailable             int
	BackorderAvailableQuantity int
	PreorderAvailableQuantity  int
	PurchaseAvailableQuantity  int
	StatusMessage              string
	ShippingMessage            string
	RawJSON                    string
}

// Availability summarises whether a coin can currently be obtained, used to
// drive the dashboard's colour-coded badge.
type Availability string

const (
	AvailableToBuy Availability = "available" // purchasable now
	PreorderOnly   Availability = "preorder"  // not yet released, preorder open
	BackorderOnly  Availability = "backorder" // out of stock, backorder open
	SoldOut        Availability = "soldout"   // nothing available
)

// Availability derives the buy status from a snapshot's quantities. Preorder
// (not yet released) takes precedence over backorder (released but out of stock).
func (s Snapshot) Availability() Availability {
	switch {
	case s.PurchaseAvailableQuantity > 0:
		return AvailableToBuy
	case s.PreorderAvailableQuantity > 0:
		return PreorderOnly
	case s.BackorderAvailableQuantity > 0:
		return BackorderOnly
	default:
		return SoldOut
	}
}

// Rank orders availability states from most to least actionable, for sorting
// the dashboard so the coins worth acting on appear first.
func (a Availability) Rank() int {
	switch a {
	case AvailableToBuy:
		return 0
	case PreorderOnly:
		return 1
	case BackorderOnly:
		return 2
	case SoldOut:
		return 3
	default:
		return 4
	}
}

// RemainingPercent reports how much of the limited edition is still available,
// as a 0-100 integer. It returns -1 when the edition size is unknown (0), so
// callers can distinguish "no bar" from "0% remaining".
func (s Snapshot) RemainingPercent() int {
	if s.LimitedEditionPresentation <= 0 {
		return -1
	}
	pct := float64(s.TotalAvailable) / float64(s.LimitedEditionPresentation) * 100
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return int(pct + 0.5)
}

// Coin is a tracked coin together with its two most recent snapshots (if any),
// so movement since the previous scrape can be reported.
type Coin struct {
	SKU       string
	Name      string
	URL       string
	FirstSeen time.Time
	LastSeen  time.Time
	Latest    *Snapshot
	Previous  *Snapshot
}

// Change reports how a coin's stock moved between the previous scrape and the
// latest one. Rates are normalised to a per-day basis because the interval
// between scrapes is not fixed.
func (c Coin) Change() *Change {
	if c.Latest == nil || c.Previous == nil {
		return nil
	}
	return &Change{
		Since:      c.Latest.CapturedAt.Sub(c.Previous.CapturedAt),
		TotalDelta: c.Latest.TotalAvailable - c.Previous.TotalAvailable,
		NowTotal:   c.Latest.TotalAvailable,
		Prev:       c.Previous.Availability(),
		Now:        c.Latest.Availability(),
	}
}

// Change is the movement in available stock between two snapshots.
type Change struct {
	Since      time.Duration // elapsed time between the two scrapes
	TotalDelta int           // TotalAvailable now minus previous (negative = stock fell)
	NowTotal   int           // current TotalAvailable
	Prev       Availability  // availability at the previous scrape
	Now        Availability  // availability at the latest scrape
}

// PerDay is the signed rate of change in available stock per 24 hours. It
// returns 0 when the interval is unknown or zero.
func (c Change) PerDay() float64 {
	if c.Since <= 0 {
		return 0
	}
	return float64(c.TotalDelta) / c.Since.Hours() * 24
}

// SoldPerDay is the rate at which stock is being consumed (positive when stock
// is falling, 0 otherwise).
func (c Change) SoldPerDay() float64 {
	if p := c.PerDay(); p < 0 {
		return -p
	}
	return 0
}

// EstSelloutDays projects how many days until stock reaches zero at the current
// rate of decline, or -1 when stock is not declining or the interval is unknown.
func (c Change) EstSelloutDays() float64 {
	s := c.SoldPerDay()
	if s <= 0 || c.NowTotal <= 0 {
		return -1
	}
	return float64(c.NowTotal) / s
}

// AvailabilityChanged reports whether the buy status changed between scrapes.
func (c Change) AvailabilityChanged() bool { return c.Prev != c.Now }

// Direction is "down", "up", or "flat" — used to colour the trend in the UI.
func (c Change) Direction() string {
	switch {
	case c.TotalDelta < 0:
		return "down"
	case c.TotalDelta > 0:
		return "up"
	default:
		return "flat"
	}
}

// DeltaText renders the raw movement, e.g. "▼ 312", "▲ 50", or "no change".
func (c Change) DeltaText() string {
	switch {
	case c.TotalDelta < 0:
		return fmt.Sprintf("▼ %d", -c.TotalDelta)
	case c.TotalDelta > 0:
		return fmt.Sprintf("▲ %d", c.TotalDelta)
	default:
		return "no change"
	}
}

// RateText renders the per-day rate, e.g. "≈740/day", or "" when negligible or
// the interval is unknown.
func (c Change) RateText() string {
	if c.Since <= 0 {
		return ""
	}
	r := math.Abs(c.PerDay())
	if r < 0.5 {
		return ""
	}
	return fmt.Sprintf("≈%d/day", int(r+0.5))
}

// SelloutText renders the projected time to sell out, or "" if not declining.
func (c Change) SelloutText() string {
	d := c.EstSelloutDays()
	switch {
	case d < 0:
		return ""
	case d < 1:
		return "≈<1d to sell out"
	default:
		return fmt.Sprintf("≈%dd to sell out", int(d+0.5))
	}
}

// SinceText renders the interval since the previous scrape, e.g. "over 5h".
func (c Change) SinceText() string {
	if c.Since <= 0 {
		return ""
	}
	if h := c.Since.Hours(); h < 1 {
		return "under 1h"
	} else if h < 48 {
		return fmt.Sprintf("over %dh", int(h))
	} else {
		return fmt.Sprintf("over %dd", int(h/24))
	}
}

// Summary is a one-line trend description for CLI output.
func (c Change) Summary() string {
	parts := []string{c.DeltaText()}
	if r := c.RateText(); r != "" {
		parts = append(parts, r)
	}
	if s := c.SelloutText(); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}
