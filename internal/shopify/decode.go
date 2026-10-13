package shopify

import (
	"encoding/json"
	"fmt"
)

// DecodeProducts reads the products the collector sends: the concatenated
// "products" arrays of a store's /products.json pages.
//
// The v1 encoding/json API is used on purpose. Go 1.27 backs it with v2,
// whose own defaults reject duplicate object names and invalid UTF-8; the
// v1 API keeps accepting both, so one sloppy store can't stop an ingest.
func DecodeProducts(body []byte) ([]Product, error) {
	var page struct {
		Products []Product `json:"products"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("decode products: %w", err)
	}
	return page.Products, nil
}
