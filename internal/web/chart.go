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

func escapeAttr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;").Replace(s)
}
