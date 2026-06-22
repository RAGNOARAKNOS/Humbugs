// Package web serves the Humbugs dashboard: a table of tracked coins with
// colour-coded availability and per-coin history charts, backed by the store.
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ragnoaraknos/Humbugs/internal/model"
	"github.com/ragnoaraknos/Humbugs/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Server renders the dashboard from a store.
type Server struct {
	store *store.Store
	tmpl  *template.Template
}

// NewServer parses the embedded templates and returns a dashboard server.
func NewServer(st *store.Store) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"statusLabel": statusLabel,
		"barClass":    barClass,
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Server{store: st, tmpl: tmpl}, nil
}

func statusLabel(a model.Availability) string {
	switch a {
	case model.AvailableToBuy:
		return "Available"
	case model.PreorderOnly:
		return "Preorder"
	case model.BackorderOnly:
		return "Backorder"
	case model.SoldOut:
		return "Sold out"
	default:
		return "Unknown"
	}
}

// barClass colours the scarcity bar amber/red as the edition runs low.
func barClass(remainingPct int) string {
	switch {
	case remainingPct <= 5:
		return "vlow"
	case remainingPct <= 25:
		return "low"
	default:
		return ""
	}
}

// Handler returns the configured HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/coin/", s.handleCoin)
	mux.HandleFunc("/api/coin/", s.handleAPIHistory)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	coins, err := s.store.Coins()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sortByActionability(coins)
	s.render(w, "index.html", map[string]any{"Coins": coins})
}

func (s *Server) handleCoin(w http.ResponseWriter, r *http.Request) {
	sku := strings.TrimPrefix(r.URL.Path, "/coin/")
	coin, err := s.findCoin(sku)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if coin == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "coin.html", map[string]any{
		"Coin":    coin,
		"APIPath": "/api/coin/" + sku + "/history",
	})
}

func (s *Server) handleAPIHistory(w http.ResponseWriter, r *http.Request) {
	// path: /api/coin/{sku}/history
	rest := strings.TrimPrefix(r.URL.Path, "/api/coin/")
	sku, _, _ := strings.Cut(rest, "/")
	if sku == "" {
		http.NotFound(w, r)
		return
	}
	snaps, err := s.store.History(sku)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type point struct {
		CapturedAt     string  `json:"captured_at"`
		Price          float64 `json:"price"`
		TotalAvailable int     `json:"total_available"`
		PreorderQty    int     `json:"preorder_qty"`
		PurchaseQty    int     `json:"purchase_qty"`
	}
	out := make([]point, 0, len(snaps))
	for _, sn := range snaps {
		out = append(out, point{
			CapturedAt:     sn.CapturedAt.Format(time.RFC3339),
			Price:          sn.Price,
			TotalAvailable: sn.TotalAvailable,
			PreorderQty:    sn.PreorderAvailableQuantity,
			PurchaseQty:    sn.PurchaseAvailableQuantity,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// sortByActionability orders coins so the ones worth acting on come first:
// by availability (buyable → preorder → backorder → sold out → no data), then
// scarcest first (lowest remaining %), then by name.
func sortByActionability(coins []model.Coin) {
	rank := func(c model.Coin) int {
		if c.Latest == nil {
			return 5
		}
		return c.Latest.Availability().Rank()
	}
	remaining := func(c model.Coin) int {
		if c.Latest == nil {
			return 101 // sort unknown after any real percentage
		}
		p := c.Latest.RemainingPercent()
		if p < 0 {
			return 101
		}
		return p
	}
	sort.SliceStable(coins, func(i, j int) bool {
		if ri, rj := rank(coins[i]), rank(coins[j]); ri != rj {
			return ri < rj
		}
		if pi, pj := remaining(coins[i]), remaining(coins[j]); pi != pj {
			return pi < pj
		}
		return coins[i].Name < coins[j].Name
	})
}

func (s *Server) findCoin(sku string) (*model.Coin, error) {
	coins, err := s.store.Coins()
	if err != nil {
		return nil, err
	}
	for i := range coins {
		if coins[i].SKU == sku {
			return &coins[i], nil
		}
	}
	return nil, nil
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
