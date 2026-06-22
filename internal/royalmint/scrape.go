// Package royalmint fetches Royal Mint product pages and extracts the stock data
// embedded in the page's data-product-settings attribute — the same source the
// original bookmarklet reads.
package royalmint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/ragnoaraknos/Humbugs/internal/model"
)

// userAgent identifies Humbugs politely rather than masquerading as a browser.
const userAgent = "Humbugs/0.1 (+https://github.com/ragnoaraknos/Humbugs) coin stock tracker"

// Client fetches and parses Royal Mint product pages.
type Client struct {
	HTTP *http.Client
}

// NewClient returns a Client with a sensible default timeout.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Result pairs the parsed product with the raw settings JSON, captured so that
// fields Humbugs does not yet model are still preserved in the snapshot.
type Result struct {
	Product model.Product
	RawJSON string
}

// Scrape fetches a product page and extracts its data-product-settings payload.
func (c *Client) Scrape(ctx context.Context, url string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("fetch %q: unexpected status %s", url, resp.Status)
	}
	return ParseReader(resp.Body)
}

// ParseReader extracts the product settings from an HTML document body. It is
// separated from Scrape so it can be exercised offline against saved fixtures.
func ParseReader(r io.Reader) (*Result, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	return parseDocument(doc)
}

func parseDocument(doc *goquery.Document) (*Result, error) {
	sel := doc.Find("div[data-product-settings]").First()
	if sel.Length() == 0 {
		return nil, fmt.Errorf("no div[data-product-settings] element found")
	}
	raw, ok := sel.Attr("data-product-settings")
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("data-product-settings attribute is empty")
	}

	var product model.Product
	if err := json.Unmarshal([]byte(raw), &product); err != nil {
		return nil, fmt.Errorf("decode product settings json: %w", err)
	}
	if product.SKU == "" {
		return nil, fmt.Errorf("product settings json has no sku")
	}
	return &Result{Product: product, RawJSON: raw}, nil
}
