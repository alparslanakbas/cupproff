package web

import (
	"fmt"
	"strings"
	"time"
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
	return fmt.Sprintf(`<svg class="spark" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="%s">`+
		`<path d="%s" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>`,
		width, height, width, height, escapeAttr(label), b.String())
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
func historyChart(points []pricePoint, now time.Time, usual int64, currency string, label, id string) string {
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
		fmt.Fprintf(&b, `<text class="axis" x="-10" y="%.1f%%" text-anchor="end" dominant-baseline="middle">%s</text>`, 100*share(v), money(v, currency))
	}
	fmt.Fprintf(&b, `<circle class="dot" cx="100%%" cy="%.1f%%" r="4.5"/>`, 100*share(points[len(points)-1].Minor))
	fmt.Fprintf(&b, `<text class="axis" x="0" y="100%%" dy="20">%s</text>`, start.UTC().Format("Jan 2"))
	fmt.Fprintf(&b, `<text class="axis" x="100%%" y="100%%" dy="20" text-anchor="end">%s</text>`, end.UTC().Format("Jan 2"))
	b.WriteString(`</svg>`)
	return b.String()
}

func escapeAttr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}
