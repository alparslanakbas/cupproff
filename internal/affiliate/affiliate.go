// Package affiliate turns a store's address into the affiliate link for that
// store, the way the sister site WheyProof does (AffiliateLinkBuilder.cs).
//
// A store gets a rule here only once its programme has approved us: a link
// through a programme that hasn't is refused or earns nothing. Stores without a
// rule go through the network fallback when one is configured (Sovrn, once our
// site is approved there), otherwise their links stay as they are. The shopper
// reaching the store comes before the click being tracked: anything that
// doesn't form a valid link returns the address unchanged.
package affiliate

import (
	"net/url"
	"strings"
)

// awinPublisher is the Awin account coffeeproof.org is a promotional space of
// (the WheyProof account; the space was added on 2026-11-24).
const awinPublisher = "3090545"

const placeholder = "{url}"

// Rule sends one store's clicks through a network.
type Rule struct {
	Host string // the store's host, with or without www
	// Link is a redirect template containing {url} (Awin, Sovrn) or a query
	// pair appended to the store address ("ref=123").
	Link string
}

// awin is the Awin deep link for an advertiser. clickref marks CoffeeProof's
// clicks: the account is shared with WheyProof, and its reports would mix them.
func awin(advertiser string) string {
	return "https://www.awin1.com/cread.php?awinmid=" + advertiser + "&awinaffid=" + awinPublisher +
		"&clickref=coffeeproof&ued=" + placeholder
}

// Rules are the programmes that have approved us.
var Rules = []Rule{
	// Intelligentsia Coffee, Awin 35273: approved 2026-11-25 (US; 4% existing
	// customers, 20% new customers, 25% a new subscriber's first order).
	{Host: "intelligentsia.com", Link: awin("35273")},
}

// Links applies the rules, then the fallback.
type Links struct {
	rules    map[string]string
	fallback string
}

// New builds the link maker. fallback is a redirect template with {url} for
// stores without a rule; empty turns it off.
func New(rules []Rule, fallback string) *Links {
	l := &Links{rules: map[string]string{}}
	for _, r := range rules {
		if h, link := normalizeHost(r.Host), strings.TrimSpace(r.Link); h != "" && link != "" {
			l.rules[h] = link
		}
	}
	if f := strings.TrimSpace(fallback); strings.HasPrefix(f, "https://") && strings.Contains(f, placeholder) {
		l.fallback = f
	}
	return l
}

// Apply returns the affiliate link for a store address, or the address as it
// is. A nil *Links leaves every address alone.
func (l *Links) Apply(address string) string {
	if l == nil {
		return address
	}
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return address
	}
	rule, ok := l.rules[normalizeHost(u.Hostname())]
	if !ok {
		// Only a redirect makes sense for a store we know nothing about; a query
		// pair is store-specific.
		if l.fallback == "" {
			return address
		}
		return redirect(l.fallback, address)
	}
	if strings.Contains(rule, placeholder) {
		return redirect(rule, address)
	}
	return appendPair(address, rule)
}

// Hosts lists the stores with a rule, for the startup log.
func (l *Links) Hosts() []string {
	hosts := make([]string, 0, len(l.rules))
	for h := range l.rules {
		hosts = append(hosts, h)
	}
	return hosts
}

// HasFallback reports whether stores without a rule go through a network.
func (l *Links) HasFallback() bool { return l != nil && l.fallback != "" }

// A redirect template must itself be an https address; anything else is a
// configuration mistake, not a link to send people to.
func redirect(template, address string) string {
	if !strings.HasPrefix(template, "https://") {
		return address
	}
	return strings.Replace(template, placeholder, url.QueryEscape(address), 1)
}

func appendPair(address, pair string) string {
	name, _, ok := strings.Cut(pair, "=")
	if !ok || name == "" || strings.HasSuffix(pair, "=") {
		return address
	}
	base, fragment, _ := strings.Cut(address, "#")
	// The same parameter twice would leave the store guessing which one counts.
	if strings.Contains(base, "?"+name+"=") || strings.Contains(base, "&"+name+"=") {
		return address
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	out := base + sep + pair
	if fragment != "" {
		out += "#" + fragment
	}
	return out
}

func normalizeHost(host string) string {
	h := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return strings.TrimPrefix(h, "www.")
}
