package web

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/alparslanakbas/cupproof/internal/i18n"
)

// pricePoint is one stored observation.
type pricePoint struct {
	At    time.Time
	Minor int64
}

// sparkline draws a price history as a step line in inline SVG: a price
// holds until the next observation changes it, so straight slopes between
// readings would show prices nobody ever saw. It returns "" when there are
// fewer than two points, because one point is not a history.
//
// The SVG carries only numbers we format ourselves, so it is safe to emit
// as raw HTML.
func sparkline(points []pricePoint, now time.Time, width, height int, label string) string {
	d, _, ok := stepPath(points, now, width, height)
	if !ok {
		return ""
	}
	return fmt.Sprintf(`<svg class="spark" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="%s">`+
		`<path d="%s" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`,
		width, height, width, height, escapeAttr(label), d)
}

// sparkArea is sparkline with the area under the line tinted, as the
// sister sites draw their small charts; the same "" rule.
func sparkArea(points []pricePoint, now time.Time, width, height int, label string) string {
	d, x0, ok := stepPath(points, now, width, height)
	if !ok {
		return ""
	}
	return fmt.Sprintf(`<svg class="spark spark-area" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="%s">`+
		`<path d="%s V%d H%.1f Z" fill="currentColor" fill-opacity=".13" stroke="none"/>`+
		`<path d="%s" fill="none" stroke="currentColor" stroke-width="2"/></svg>`,
		width, height, width, height, escapeAttr(label), d, height, x0, d)
}

// stepPath is the step line's SVG path and its first x; ok is false with
// fewer than two points.
func stepPath(points []pricePoint, now time.Time, width, height int) (d string, x0 float64, ok bool) {
	if len(points) < 2 {
		return "", 0, false
	}
	start, end := points[0].At, now
	if !end.After(start) {
		end = start.Add(time.Hour)
	}
	lo, hi := points[0].Minor, points[0].Minor
	for _, p := range points {
		lo, hi = min(lo, p.Minor), max(hi, p.Minor)
	}
	const pad = 3.0
	w, h := float64(width), float64(height)
	x := func(t time.Time) float64 {
		return pad + (w-2*pad)*float64(t.Sub(start))/float64(end.Sub(start))
	}
	y := func(v int64) float64 {
		if hi == lo {
			return h / 2
		}
		return pad + (h-2*pad)*float64(hi-v)/float64(hi-lo)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M%.1f %.1f", x(points[0].At), y(points[0].Minor))
	for i := 1; i < len(points); i++ {
		fmt.Fprintf(&b, "H%.1f V%.1f", x(points[i].At), y(points[i].Minor))
	}
	fmt.Fprintf(&b, "H%.1f", x(end))
	return b.String(), x(points[0].At), true
}

// historyChart is the bigger chart of a product page and of a listing's quick
// look: the same step line with the area under it, the usual price as a
// dashed line when there is one, the price range on the left and the first
// and last day under it. It returns "" with fewer than two points, like
// sparkline.
//
// The CSS gives the chart its height and the column its width. The plot is a
// nested SVG stretched to that box (preserveAspectRatio="none", strokes that
// don't scale); the labels and today's dot sit outside it at percentages of
// the box, so they keep their size. A fixed viewBox for the whole chart drew
// 4 px labels on a phone and 16 px ones on a desktop. id keeps the
// gradient's id unique on the page.
func historyChart(lang string, points []pricePoint, now time.Time, usual int64, currency string, label, id string) string {
	if len(points) < 2 {
		return ""
	}
	start, end := points[0].At, now
	if !end.After(start) {
		end = start.Add(time.Hour)
	}
	lo, hi := points[0].Minor, points[0].Minor
	for _, p := range points {
		lo, hi = min(lo, p.Minor), max(hi, p.Minor)
	}
	if usual > 0 {
		lo, hi = min(lo, usual), max(hi, usual)
	}
	// Some air above and below, so a flat price isn't drawn on the frame.
	span := max(hi-lo, max(hi/20, 1))
	top, bottom := hi+span/3, max(lo-span/3, 0)
	const w, h = 1000.0, 300.0 // the plot's own units; the CSS stretches them
	x := func(t time.Time) float64 {
		return w * float64(t.Sub(start)) / float64(end.Sub(start))
	}
	share := func(v int64) float64 { // 0 at the top, 1 at the bottom
		return float64(top-v) / float64(top-bottom)
	}
	var line strings.Builder
	fmt.Fprintf(&line, "M%.1f %.1f", x(points[0].At), h*share(points[0].Minor))
	for i := 1; i < len(points); i++ {
		fmt.Fprintf(&line, "H%.1f V%.1f", x(points[i].At), h*share(points[i].Minor))
	}
	fmt.Fprintf(&line, "H%.1f", w)
	gid := "fill-" + escapeAttr(id)
	var b strings.Builder
	data := readings(lang, points,
		func(t time.Time) float64 { return 100 * x(t) / w },
		func(v int64) float64 { return 100 * share(v) },
		currency)
	fmt.Fprintf(&b, `<div class="chart-box" data-points="%s">`, escapeAttr(data))
	fmt.Fprintf(&b, `<svg class="chart" role="img" aria-label="%s">`, escapeAttr(label))
	fmt.Fprintf(&b, `<svg class="plot" viewBox="0 0 %.0f %.0f" preserveAspectRatio="none" width="100%%" height="100%%" aria-hidden="true">`, w, h)
	fmt.Fprintf(&b, `<defs><linearGradient id="%s" x1="0" y1="0" x2="0" y2="1">`+
		`<stop offset="0" stop-color="currentColor" stop-opacity=".28"/><stop offset="1" stop-color="currentColor" stop-opacity="0"/>`+
		`</linearGradient></defs>`, gid)
	if usual > 0 {
		fmt.Fprintf(&b, `<line class="usual" x1="0" x2="%.0f" y1="%.1f" y2="%.1f" vector-effect="non-scaling-stroke"/>`, w, h*share(usual), h*share(usual))
	}
	fmt.Fprintf(&b, `<path class="area" d="%sV%.0f H0 Z" fill="url(#%s)"/>`, line.String(), h, gid)
	fmt.Fprintf(&b, `<path class="line" d="%s" fill="none" stroke="currentColor" vector-effect="non-scaling-stroke"/>`, line.String())
	b.WriteString(`</svg>`)
	labels := []int64{hi}
	if lo != hi {
		labels = append(labels, lo)
	}
	for _, v := range labels {
		fmt.Fprintf(&b, `<text class="axis" x="-10" y="%.1f%%" text-anchor="end" dominant-baseline="middle">%s</text>`, 100*share(v), money(lang, v, currency))
	}
	fmt.Fprintf(&b, `<circle class="dot" cx="100%%" cy="%.1f%%" r="4.5"/>`, 100*share(points[len(points)-1].Minor))
	fmt.Fprintf(&b, `<text class="axis" x="0" y="100%%" dy="20">%s</text>`, i18n.Day(lang, start))
	fmt.Fprintf(&b, `<text class="axis" x="100%%" y="100%%" dy="20" text-anchor="end">%s</text>`, i18n.Day(lang, end))
	// The readout's guide and dot; the script puts them on the reading nearest
	// the pointer and fills the tag below.
	b.WriteString(`<line class="cursor" x1="0" x2="0" y1="0" y2="100%"/><circle class="cursor-dot" cx="0" cy="0" r="4.5"/>`)
	b.WriteString(`</svg><div class="chart-tip" hidden><div class="chart-tip-date"></div><div class="chart-tip-price"></div></div></div>`)
	return b.String()
}

// readings are the chart's readings as its script reads them: x and y as
// percentages of the box, the date and the price. A pointer or a finger picks
// the nearest one and a guide, a dot and a tag follow it, as on the sister
// site (frontend core/chart-hover.ts; here static/site.js). Two readings on
// one day carry the time, so the tag tells them apart.
func readings(lang string, points []pricePoint, xPct func(time.Time) float64, yPct func(int64) float64, currency string) string {
	n := len(points)
	rows := make([][4]any, 0, n)
	for i, p := range points {
		date := i18n.Date(lang, p.At)
		if (i > 0 && sameDay(points[i-1].At, p.At)) || (i < n-1 && sameDay(points[i+1].At, p.At)) {
			date = i18n.DayTime(lang, p.At) + " UTC"
		}
		rows = append(rows, [4]any{math.Round(xPct(p.At)*100) / 100, math.Round(yPct(p.Minor)*100) / 100, date,
			money(lang, p.Minor, currency)})
	}
	b, _ := json.Marshal(rows)
	return string(b)
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func escapeAttr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}
