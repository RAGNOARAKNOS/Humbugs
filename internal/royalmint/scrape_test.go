package royalmint

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ragnoaraknos/Humbugs/internal/model"
)

func TestParseReader(t *testing.T) {
	f, err := os.Open("testdata/product.html")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	res, err := ParseReader(f)
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}

	p := res.Product
	if p.SKU != "UK24GS" {
		t.Errorf("SKU = %q, want UK24GS", p.SKU)
	}
	if p.ProductName != "2024 Gold Proof Sovereign" {
		t.Errorf("ProductName = %q", p.ProductName)
	}
	if p.Price != "620.00" {
		t.Errorf("Price = %q, want \"620.00\"", p.Price)
	}
	if p.PriceValue() != 620.00 {
		t.Errorf("PriceValue() = %v, want 620.00", p.PriceValue())
	}
	if p.CurrentPrice != "£620.00" {
		t.Errorf("CurrentPrice = %q, want \"£620.00\"", p.CurrentPrice)
	}
	if got, want := int(p.StockSummary.PurchaseAvailableQuantity), 1150; got != want {
		t.Errorf("PurchaseAvailableQuantity = %d, want %d", got, want)
	}
	if got, want := int(p.StockSummary.LimitedEditionPresentation), 7495; got != want {
		t.Errorf("LimitedEditionPresentation = %d, want %d", got, want)
	}
	if p.StockSummary.StatusMessage != "Available to Order" {
		t.Errorf("StatusMessage = %q", p.StockSummary.StatusMessage)
	}
	if p.StockSummary.ShippingMessage != "Shipping early August" {
		t.Errorf("ShippingMessage = %q", p.StockSummary.ShippingMessage)
	}
	if !strings.Contains(res.RawJSON, "stockSummary") {
		t.Errorf("RawJSON not captured: %q", res.RawJSON)
	}
}

func TestParseReaderMissingElement(t *testing.T) {
	_, err := ParseReader(strings.NewReader("<html><body>no settings here</body></html>"))
	if err == nil {
		t.Fatal("expected error for missing data-product-settings element")
	}
}

func TestFlexIntDecoding(t *testing.T) {
	// The Royal Mint encodes quantities inconsistently; all must parse.
	const doc = `{"stockSummary":{"TotalAvailable":6252.0,"BackorderAvailableQuantity":"42","PreorderAvailableQuantity":7,"PurchaseAvailableQuantity":null,"LimitedEditionPresentation":12500}}`
	var p model.Product
	if err := json.Unmarshal([]byte(doc), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ss := p.StockSummary
	if ss.TotalAvailable != 6252 {
		t.Errorf("TotalAvailable (float) = %d, want 6252", ss.TotalAvailable)
	}
	if ss.BackorderAvailableQuantity != 42 {
		t.Errorf("BackorderAvailableQuantity (string) = %d, want 42", ss.BackorderAvailableQuantity)
	}
	if ss.PreorderAvailableQuantity != 7 {
		t.Errorf("PreorderAvailableQuantity (int) = %d, want 7", ss.PreorderAvailableQuantity)
	}
	if ss.PurchaseAvailableQuantity != 0 {
		t.Errorf("PurchaseAvailableQuantity (null) = %d, want 0", ss.PurchaseAvailableQuantity)
	}
}

func TestRemainingPercent(t *testing.T) {
	cases := []struct {
		name string
		snap model.Snapshot
		want int
	}{
		{"half", model.Snapshot{TotalAvailable: 50, LimitedEditionPresentation: 100}, 50},
		{"rounds", model.Snapshot{TotalAvailable: 1, LimitedEditionPresentation: 3}, 33},
		{"none left", model.Snapshot{TotalAvailable: 0, LimitedEditionPresentation: 100}, 0},
		{"unknown edition", model.Snapshot{TotalAvailable: 5, LimitedEditionPresentation: 0}, -1},
		{"caps at 100", model.Snapshot{TotalAvailable: 120, LimitedEditionPresentation: 100}, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.snap.RemainingPercent(); got != tc.want {
				t.Errorf("RemainingPercent() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestChange(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	coin := model.Coin{
		Previous: &model.Snapshot{CapturedAt: t0, TotalAvailable: 1000, PurchaseAvailableQuantity: 10},
		Latest:   &model.Snapshot{CapturedAt: t0.Add(12 * time.Hour), TotalAvailable: 880, BackorderAvailableQuantity: 880},
	}
	ch := coin.Change()
	if ch == nil {
		t.Fatal("Change() = nil, want non-nil")
	}
	if ch.TotalDelta != -120 {
		t.Errorf("TotalDelta = %d, want -120", ch.TotalDelta)
	}
	// -120 over 12h normalises to -240/day.
	if got := ch.PerDay(); math.Abs(got-(-240)) > 1e-9 {
		t.Errorf("PerDay() = %v, want -240", got)
	}
	if got := ch.SoldPerDay(); math.Abs(got-240) > 1e-9 {
		t.Errorf("SoldPerDay() = %v, want 240", got)
	}
	// 880 remaining / 240 per day ≈ 3.67 days.
	if got := ch.EstSelloutDays(); math.Abs(got-3.6667) > 0.01 {
		t.Errorf("EstSelloutDays() = %v, want ~3.67", got)
	}
	if got := ch.DeltaText(); got != "▼ 120" {
		t.Errorf("DeltaText() = %q, want \"▼ 120\"", got)
	}
	if got := ch.RateText(); got != "≈240/day" {
		t.Errorf("RateText() = %q, want \"≈240/day\"", got)
	}
	if !ch.AvailabilityChanged() {
		t.Error("AvailabilityChanged() = false, want true (available → backorder)")
	}
	if ch.Direction() != "down" {
		t.Errorf("Direction() = %q, want down", ch.Direction())
	}
}

func TestChangeNoPrevious(t *testing.T) {
	coin := model.Coin{Latest: &model.Snapshot{TotalAvailable: 100}}
	if ch := coin.Change(); ch != nil {
		t.Errorf("Change() = %+v, want nil when no previous snapshot", ch)
	}
}

func TestAvailability(t *testing.T) {
	cases := []struct {
		name string
		snap model.Snapshot
		want model.Availability
	}{
		{"buyable", model.Snapshot{PurchaseAvailableQuantity: 5}, model.AvailableToBuy},
		{"preorder", model.Snapshot{PreorderAvailableQuantity: 5}, model.PreorderOnly},
		{"backorder", model.Snapshot{BackorderAvailableQuantity: 5}, model.BackorderOnly},
		{"preorder beats backorder", model.Snapshot{PreorderAvailableQuantity: 1, BackorderAvailableQuantity: 5}, model.PreorderOnly},
		{"soldout", model.Snapshot{}, model.SoldOut},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.snap.Availability(); got != tc.want {
				t.Errorf("Availability() = %q, want %q", got, tc.want)
			}
		})
	}
}
