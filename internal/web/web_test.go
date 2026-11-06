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

// The history chart's labels and dot must not scale with the plot: they sit
// outside the stretched SVG at percentages of the box, so a phone gets the
// same 12 px text as a desktop.
func TestHistoryChartKeepsLabelsOutOfThePlot(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if got := historyChart([]pricePoint{{day, 2000}}, day.Add(24*time.Hour), 0, "USD", "x", "1"); got != "" {
		t.Fatalf("one point drew %q", got)
	}
	pts := []pricePoint{{day, 2000}, {day.Add(48 * time.Hour), 1800}}
	svg := historyChart(pts, day.Add(96*time.Hour), 2000, "USD", "1 kg price", "size-1")
	plot := svg[strings.Index(svg, `<svg class="plot"`):strings.Index(svg, `</svg>`)]
	if !strings.Contains(plot, `preserveAspectRatio="none"`) {
		t.Errorf("plot doesn't stretch: %s", plot)
	}
	if strings.Contains(plot, "<text") || strings.Contains(plot, "<circle") {
		t.Errorf("labels or dot inside the stretched plot would be distorted: %s", plot)
	}
	// $20.00 is the top label (usual and first price), $18.00 the bottom one.
	for _, want := range []string{`>$20.00</text>`, `>$18.00</text>`, `>Oct 1</text>`, `>Oct 5</text>`, `cx="100%"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("chart lacks %s: %s", want, svg)
		}
	}
}

// Hovering the chart reads a reading without a script (6 Nov, reported
// missing): one strip per reading, the first from the left edge and the last
// to the right edge, each tag with the reading's date and price. Near the left
// edge the tag leans right; near the top it hangs below the point.
func TestHistoryChartReadings(t *testing.T) {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	pts := []pricePoint{{day, 2000}, {day.Add(24 * time.Hour), 2000}, {day.Add(72 * time.Hour), 1800}}
	svg := historyChart(pts, day.Add(96*time.Hour), 0, "USD", "x", "size-1")
	if got := strings.Count(svg, `<g class="hit">`); got != 3 {
		t.Fatalf("%d strips for 3 readings: %s", got, svg)
	}
	for _, want := range []string{
		`<rect class="zone" x="0.00%" y="0" width="12.50%"`,  // halfway to the second reading at 25 %
		`<rect class="zone" x="50.00%" y="0" width="50.00%"`, // the last reading answers up to today
		`>Oct 1, 2026</text>`,
		`text-anchor="start">$20.00</text>`, // at the left edge
		`>Oct 4, 2026</text>`,
		`text-anchor="middle">$18.00</text>`,
		`<rect x="-10" y="12" width="116"`, // $20.00 is near the top: the tag hangs below
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("chart lacks %s: %s", want, svg)
		}
	}
	twice := historyChart([]pricePoint{{day, 2000}, {day.Add(12 * time.Hour), 1900}}, day.Add(24*time.Hour), 0, "USD", "x", "size-1")
	for _, want := range []string{`>Oct 1, 09:00 UTC</text>`, `>Oct 1, 21:00 UTC</text>`} {
		if !strings.Contains(twice, want) {
			t.Errorf("two readings on one day lack the time %s: %s", want, twice)
		}
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

func TestLocalPathStaysOnSite(t *testing.T) {
	for in, want := range map[string]string{
		"/en-us/?cat=decaf&page=2":      "/en-us/?cat=decaf&page=2",
		"/en-us/coffee/onyx-geo%20":     "/en-us/coffee/onyx-geo%20",
		"/":                             "/",
		"":                              "/",
		"en-us/":                        "/",
		"https://evil.example/":         "/",
		"//evil.example/":               "/",
		"/\\evil.example/":              "/",
		"/\t/evil.example/":             "/", // browsers drop the tab: "//evil.example/"
		"/\n/evil.example/":             "/",
		"/\r\n/evil.example/":           "/",
		"/ /evil.example/":              "/",
		"/a\\b":                         "/",
		"/café":                         "/",
		"/" + strings.Repeat("a", 1000): "/",
	} {
		if got := localPath(in); got != want {
			t.Errorf("localPath(%q) = %q, want %q", in, got, want)
		}
	}
}
