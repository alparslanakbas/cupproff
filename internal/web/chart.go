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
	readings(&b, points,
		func(t time.Time) float64 { return 100 * x(t) / w },
		func(v int64) float64 { return 100 * share(v) },
		currency)
	b.WriteString(`</svg>`)
	return b.String()
}

// readings lets a pointer read the chart: each reading owns the strip of the
// box nearest to it (split halfway to its neighbours), and hovering or tapping
// the strip shows a guide, a dot and a tag with the reading's date and price.
// The sister sites' charts pick the nearest point the same way with a script;
// here it is SVG and CSS, so the page still runs none. Positions are
// percentages of the box, like the labels; the tag is a nested SVG placed at
// the point, so its pixel offsets don't stretch. Near an edge the tag leans
// inward and near the top it hangs below the point, so it is never clipped.
func readings(b *strings.Builder, points []pricePoint, xPct func(time.Time) float64, yPct func(int64) float64, currency string) {
	n := len(points)
	for i, p := range points {
		px, py := xPct(p.At), yPct(p.Minor)
		left, right := 0.0, 100.0
		if i > 0 {
			left = (xPct(points[i-1].At) + px) / 2
		}
		if i < n-1 {
			right = (px + xPct(points[i+1].At)) / 2
		}
		if right <= left {
			continue
		}
		date := p.At.UTC().Format("Jan 2, 2006")
		if (i > 0 && sameDay(points[i-1].At, p.At)) || (i < n-1 && sameDay(points[i+1].At, p.At)) {
			date = p.At.UTC().Format("Jan 2, 15:04 UTC") // two readings that day: the time tells them apart
		}
		rectX, anchor := -58.0, "middle"
		switch {
		case px < 12:
			rectX, anchor = -10, "start"
		case px > 88:
			rectX, anchor = -106, "end"
		}
		rectY, dateY, priceY := -54.0, -37.0, -20.0
		if py < 30 {
			rectY, dateY, priceY = 12, 29, 46
		}
		fmt.Fprintf(b, `<g class="hit"><rect class="zone" x="%.2f%%" y="0" width="%.2f%%" height="100%%"/>`, left, right-left)
		fmt.Fprintf(b, `<g class="tip"><line x1="%.2f%%" x2="%.2f%%" y1="0" y2="100%%"/><circle cx="%.2f%%" cy="%.2f%%" r="4.5"/>`, px, px, px, py)
		fmt.Fprintf(b, `<svg x="%.2f%%" y="%.2f%%" overflow="visible"><rect x="%.0f" y="%.0f" width="116" height="42" rx="8"/>`, px, py, rectX, rectY)
		fmt.Fprintf(b, `<text y="%.0f" text-anchor="%s">%s</text><text class="v" y="%.0f" text-anchor="%s">%s</text></svg></g></g>`,
			dateY, anchor, date, priceY, anchor, money(p.Minor, currency))
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func escapeAttr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}
