// Command humbugs tracks Royal Mint coin stock levels over time.
//
// Usage:
//
//	humbugs scrape [--config coins.yaml] [--db humbugs.db]          one-shot scrape pass
//	humbugs serve  [--db humbugs.db] [--port 8080 | --addr :8080]   dashboard web server
//	humbugs list   [--db humbugs.db]                                print tracked coins
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ragnoaraknos/Humbugs/internal/config"
	"github.com/ragnoaraknos/Humbugs/internal/model"
	"github.com/ragnoaraknos/Humbugs/internal/royalmint"
	"github.com/ragnoaraknos/Humbugs/internal/store"
	"github.com/ragnoaraknos/Humbugs/internal/web"
)

// politeDelay is the pause between consecutive page fetches in a scrape pass.
const politeDelay = 3 * time.Second

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "scrape":
		err = runScrape(args)
	case "serve":
		err = runServe(args)
	case "list":
		err = runList(args)
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "humbugs: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "humbugs: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `humbugs — Royal Mint coin stock tracker

Usage:
  humbugs scrape [--config coins.yaml] [--db humbugs.db]   one-shot scrape pass
  humbugs serve  [--db humbugs.db] [--port 8080 | --addr :8080]   dashboard web server
  humbugs list   [--db humbugs.db]                         print tracked coins

The dashboard port can also be set via the $HUMBUGS_ADDR environment variable.
`)
}

func runScrape(args []string) error {
	fs := flag.NewFlagSet("scrape", flag.ExitOnError)
	cfgPath := fs.String("config", "coins.yaml", "path to coins config")
	dbPath := fs.String("db", "humbugs.db", "path to SQLite database")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	client := royalmint.NewClient()
	ctx := context.Background()
	var failures int

	for i, coin := range cfg.Coins {
		if i > 0 {
			time.Sleep(politeDelay) // be gentle on the Mint's servers
		}
		res, err := client.Scrape(ctx, coin.URL)
		if err != nil {
			failures++
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", coin.URL, err)
			continue
		}
		now := time.Now().UTC()
		name := res.Product.ProductName
		if coin.Name != "" {
			name = coin.Name // explicit label overrides the scraped name
		}
		if err := st.UpsertCoin(res.Product.SKU, name, coin.URL, now); err != nil {
			return err
		}
		snap := snapshotFrom(res, now)
		if err := st.InsertSnapshot(snap); err != nil {
			return err
		}
		fmt.Printf("  ✓ %s (%s): buy=%d preorder=%d total=%d\n",
			name, res.Product.SKU,
			snap.PurchaseAvailableQuantity, snap.PreorderAvailableQuantity,
			snap.TotalAvailable)
	}

	fmt.Printf("scrape complete: %d ok, %d failed\n", len(cfg.Coins)-failures, failures)
	if failures > 0 {
		return fmt.Errorf("%d coin(s) failed to scrape", failures)
	}
	return nil
}

func snapshotFrom(res *royalmint.Result, at time.Time) model.Snapshot {
	p := res.Product
	return model.Snapshot{
		SKU:                        p.SKU,
		CapturedAt:                 at,
		Price:                      p.PriceValue(),
		DisplayPrice:               p.CurrentPrice,
		LimitedEditionPresentation: int(p.StockSummary.LimitedEditionPresentation),
		TotalAvailable:             int(p.StockSummary.TotalAvailable),
		BackorderAvailableQuantity: int(p.StockSummary.BackorderAvailableQuantity),
		PreorderAvailableQuantity:  int(p.StockSummary.PreorderAvailableQuantity),
		PurchaseAvailableQuantity:  int(p.StockSummary.PurchaseAvailableQuantity),
		StatusMessage:              p.StockSummary.StatusMessage,
		ShippingMessage:            p.StockSummary.ShippingMessage,
		RawJSON:                    res.RawJSON,
	}
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	dbPath := fs.String("db", "humbugs.db", "path to SQLite database")
	addr := fs.String("addr", envOr("HUMBUGS_ADDR", ":8080"), "listen address (host:port); default from $HUMBUGS_ADDR")
	port := fs.Int("port", 0, "listen port (shorthand for --addr :PORT; overrides --addr)")
	fs.Parse(args)

	listen := *addr
	if *port != 0 {
		if *port < 1 || *port > 65535 {
			return fmt.Errorf("invalid --port %d: must be between 1 and 65535", *port)
		}
		listen = fmt.Sprintf(":%d", *port)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	srv, err := web.NewServer(st)
	if err != nil {
		return err
	}
	fmt.Printf("Humbugs dashboard listening on http://localhost%s\n", listen)
	return http.ListenAndServe(listen, srv.Handler())
}

// envOr returns the value of environment variable key, or fallback if unset.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	dbPath := fs.String("db", "humbugs.db", "path to SQLite database")
	fs.Parse(args)

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	coins, err := st.Coins()
	if err != nil {
		return err
	}
	if len(coins) == 0 {
		fmt.Println("No coins tracked yet. Add coins to coins.yaml and run 'humbugs scrape'.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SKU\tNAME\tSTATUS\tPRICE\tBUY\tTOTAL\tREMAIN\tTREND (since last)\tLAST CHECKED")
	for _, c := range coins {
		status, price, buy, total, remain, trend, last := "no data", "—", "—", "—", "—", "—", "—"
		if c.Latest != nil {
			status = string(c.Latest.Availability())
			if c.Latest.DisplayPrice != "" {
				price = c.Latest.DisplayPrice
			}
			buy = fmt.Sprintf("%d", c.Latest.PurchaseAvailableQuantity)
			total = fmt.Sprintf("%d", c.Latest.TotalAvailable)
			if p := c.Latest.RemainingPercent(); p >= 0 {
				remain = fmt.Sprintf("%d%%", p)
			}
			last = c.LastSeen.Format("2006-01-02 15:04")
		}
		if ch := c.Change(); ch != nil {
			trend = ch.Summary()
			if ch.AvailabilityChanged() {
				trend += fmt.Sprintf(" [%s→%s]", ch.Prev, ch.Now)
			}
		} else if c.Latest != nil {
			trend = "first check"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			c.SKU, c.Name, status, price, buy, total, remain, trend, last)
	}
	return tw.Flush()
}
