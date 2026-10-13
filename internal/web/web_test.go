package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSparklineNeedsTwoPoints(t *testing.T) {
	day := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	if got := sparkline([]pricePoint{{day, 2000}}, day.Add(24*time.Hour), 120, 32, "x"); got != "" {
		t.Fatalf("one point drew %q; one reading is not a history", got)
	}
}

// A price holds until the next reading, so the line must be made of
// horizontal and vertical steps only, never slopes.
func TestSparklineIsAStepLine(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pts := []pricePoint{{day, 2000}, {day.Add(48 * time.Hour), 1800}, {day.Add(96 * time.Hour), 2000}}
	svg := sparkline(pts, day.Add(120*time.Hour), 120, 32, `Price "history"`)
	d := svg[strings.Index(svg, `d="`)+3:]
	d = d[:strings.Index(d, `"`)]
	for _, cmd := range strings.Fields(strings.NewReplacer("M", " M", "H", " H", "V", " V").Replace(d)) {
		if c := cmd[0]; c != 'M' && c != 'H' && c != 'V' && (c < '0' || c > '9') && c != '.' {
			t.Fatalf("path %q has a non-step command %q", d, cmd)
		}
	}
	if !strings.Contains(svg, `aria-label="Price &quot;history&quot;"`) {
		t.Errorf("label not escaped: %s", svg)
	}
}

func TestMoneyAndPerKilo(t *testing.T) {
	if got := money(1925, "USD"); got != "$19.25" {
		t.Errorf("money = %q", got)
	}
	if got := perKilo(2000, 340, "USD"); got != "$58.82/kg" {
		t.Errorf("perKilo = %q", got)
	}
	if got := perKilo(2000, 0, "USD"); got != "" {
		t.Errorf("perKilo without a weight = %q, want empty (never assumed)", got)
	}
	if got := perHundredGrams(2800, 113, "USD"); got != "$24.78/100 g" {
		t.Errorf("perHundredGrams = %q", got)
	}
	if got := perHundredGrams(2800, 0, "USD"); got != "" {
		t.Errorf("perHundredGrams without a weight = %q, want empty", got)
	}
}

// The tea section's routes must coexist with /static/{file}: ServeMux
// panics at start-up on overlapping patterns, so building the handler is
// itself the test. Section slashes are added with a permanent redirect.
func TestSectionRoutes(t *testing.T) {
	h := (&Server{SiteURL: "http://x", Log: slog.New(slog.DiscardHandler)}).Handler()
	cases := map[string]int{"/en-us/tea": http.StatusMovedPermanently, "/en-us/teas": http.StatusNotFound, "/xx-yy/tea": http.StatusNotFound}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
		if want == http.StatusMovedPermanently && rec.Header().Get("Location") != "/en-us/tea/" {
			t.Errorf("%s moved to %q", path, rec.Header().Get("Location"))
		}
	}
}

func TestCDNImageResizesOnlyShopify(t *testing.T) {
	got := cdnImage("https://cdn.shopify.com/s/files/a.png?v=1", 480)
	if !strings.Contains(got, "width=480") || !strings.Contains(got, "v=1") {
		t.Errorf("shopify image = %q", got)
	}
	if other := "https://example.com/a.png"; cdnImage(other, 480) != other {
		t.Errorf("non-Shopify image changed")
	}
}
