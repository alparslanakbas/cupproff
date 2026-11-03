// Package robots answers "may this bot fetch this path?" from a robots.txt,
// following RFC 9309: the group for our product token wins over "*", and
// within a group the longest matching rule decides (Allow wins a tie).
package robots

import (
	"bufio"
	"math"
	"strconv"
	"strings"
	"time"
)

// Rules is a parsed robots.txt, reduced to the group that applies to us.
//
// Crawl-delay (3 Nov 2026) isn't in the RFC, but Shopify's bot review asked
// how the crawler handles it, and it was read past until then. The group's
// value is the least time between two requests to the store; a value that
// isn't a positive number is ignored.
type Rules struct {
	allow, disallow []string
	// Delay is the group's Crawl-delay, zero when it sets none.
	Delay time.Duration
}

// Parse keeps the rules of the group for token (case-insensitive) or, when
// there is none, the "*" group. An empty file allows all.
func Parse(body, token string) Rules {
	token = strings.ToLower(token)
	type group struct {
		agents          []string
		allow, disallow []string
		delay           time.Duration
	}
	var groups []*group
	var cur *group
	lastWasAgent := false
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if cur == nil || !lastWasAgent {
				cur = &group{}
				groups = append(groups, cur)
			}
			cur.agents = append(cur.agents, strings.ToLower(value))
			lastWasAgent = true
		case "crawl-delay":
			lastWasAgent = false
			s, err := strconv.ParseFloat(value, 64)
			if cur == nil || err != nil || !(s > 0) || math.IsInf(s, 0) {
				continue
			}
			// Capped at a day only to stay in range; that long refuses the store anyway.
			if d := time.Duration(math.Min(s, 86400) * float64(time.Second)); d > cur.delay {
				cur.delay = d
			}
		case "allow", "disallow":
			lastWasAgent = false
			if cur == nil || value == "" {
				continue
			}
			if key == "allow" {
				cur.allow = append(cur.allow, value)
			} else {
				cur.disallow = append(cur.disallow, value)
			}
		default:
			lastWasAgent = false
		}
	}
	var star *group
	for _, g := range groups {
		for _, a := range g.agents {
			if a == token {
				return Rules{g.allow, g.disallow, g.delay}
			}
			if a == "*" && star == nil {
				star = g
			}
		}
	}
	if star != nil {
		return Rules{star.allow, star.disallow, star.delay}
	}
	return Rules{}
}

// Allowed reports whether path (with its query) may be fetched.
func (r Rules) Allowed(path string) bool {
	best, allowed := -1, true
	for _, p := range r.disallow {
		if matches(p, path) && len(p) > best {
			best, allowed = len(p), false
		}
	}
	for _, p := range r.allow {
		if matches(p, path) && len(p) >= best {
			best, allowed = len(p), true
		}
	}
	return allowed
}

// All holds the rules for several product tokens: a path may be fetched only
// when every one of them allows it. Signed requests carry the sister site's
// bot name (see webbotauth), so a store's rules for that name apply too.
type All []Rules

// Allowed reports whether every set of rules allows path.
func (a All) Allowed(path string) bool {
	for _, r := range a {
		if !r.Allowed(path) {
			return false
		}
	}
	return true
}

// Delay is the longest Crawl-delay among the tokens' rules.
func (a All) Delay() time.Duration {
	var d time.Duration
	for _, r := range a {
		d = max(d, r.Delay)
	}
	return d
}

// matches implements RFC 9309 patterns: "*" matches any run of characters
// and a trailing "$" anchors the end.
func matches(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	rest := path[len(parts[0]):]
	if len(parts) == 1 {
		return !anchored || rest == ""
	}
	middle, last := parts[1:len(parts)-1], parts[len(parts)-1]
	for _, part := range middle {
		i := strings.Index(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
	}
	// An anchored pattern's last piece must sit at the very end, so it is
	// matched from the end ("/*.php$" matches "/a.php/b.php").
	if anchored {
		return strings.HasSuffix(rest, last)
	}
	return strings.Contains(rest, last)
}
