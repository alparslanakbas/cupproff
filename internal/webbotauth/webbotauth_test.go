package webbotauth

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The primitives are pinned to the RFCs' own test vectors (the same ones
// WheyProof's and ProteinAvcısı's tests use), and every signature we make is
// checked with an Ed25519 verifier: a header that is present but doesn't
// verify got a Shopify forum crawler treated as unsigned.

// RFC 9421 Appendix B.1.4, test-key-ed25519.
const (
	rfcSeed      = "n4Ni-HpISpVObnQMW0wOhCKROaIKqKtW_2ZYb2p9KcU"
	rfcPublicKey = "JrQLj5P_89iXES9-vFgrIy29clF9CC_oPPsw3c5D0bs"
	agent        = "https://api.wheyproof.com"
	ua           = "Mozilla/5.0 (compatible; wheyproofbot/1.0; +https://www.wheyproof.com/bot)"
)

var fixedNow = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)

func seed(t *testing.T) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(rfcSeed)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func signer(t *testing.T) *Signer {
	t.Helper()
	s, err := New(seed(t), agent, ua)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return fixedNow }
	return s
}

func TestSigningMatchesTheRFC9421Ed25519TestCase(t *testing.T) {
	// RFC 9421 Appendix B.2.6, line wrapping removed.
	base := SignatureBase([][2]string{
		{"date", "Tue, 20 Apr 2021 02:07:55 GMT"}, {"@method", "POST"}, {"@path", "/foo"},
		{"@authority", "example.com"}, {"content-type", "application/json"}, {"content-length", "18"},
	}, `("date" "@method" "@path" "@authority" "content-type" "content-length");created=1618884473;keyid="test-key-ed25519"`)

	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed(t)), []byte(base)))

	const want = "wqcAqbmYJ2ji2glfAMaRy4gruYYnx2nEFN2HN6jrnDnQCK1u02Gb04v9EDgwUPiu4A0w6vuQv5lIp5WPpBKRCw=="
	if sig != want {
		t.Fatalf("signature = %s, want %s", sig, want)
	}
}

func TestKeyIDIsTheRFC8037Thumbprint(t *testing.T) {
	// RFC 8037 Appendix A.3.
	if got := Thumbprint("11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"); got != "kPrK_qmxVWaYVA9wwBF6Iuo3vVzz7TxHCTwXBygrS4k" {
		t.Fatalf("thumbprint = %s", got)
	}
	if got, want := signer(t).KeyID(), Thumbprint(rfcPublicKey); got != want {
		t.Fatalf("keyid = %s, want %s", got, want)
	}
}

func TestASignedRequestCarriesTheHeadersAndVerifies(t *testing.T) {
	s := signer(t)
	r, _ := http.NewRequest(http.MethodGet, "https://Heart-Roasters.com/products.json?limit=250&page=1", nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 (compatible; CupProofBot/0.1)")

	s.Sign(r)

	gotAgent, input, sig := r.Header.Get("Signature-Agent"), r.Header.Get("Signature-Input"), r.Header.Get("Signature")
	// A quoted https URL, not a dictionary member (Cloudflare's guide: sig1="..." fails).
	if gotAgent != `"https://api.wheyproof.com"` {
		t.Fatalf("Signature-Agent = %s", gotAgent)
	}
	for _, part := range []string{`sig1=("@authority" "signature-agent");`, `;keyid="` + s.KeyID() + `"`, `;alg="ed25519"`, `;tag="web-bot-auth"`} {
		if !strings.Contains(input, part) {
			t.Fatalf("Signature-Input %q lacks %q", input, part)
		}
	}
	created, _ := strconv.ParseInt(regexp.MustCompile(`;created=(\d+)`).FindStringSubmatch(input)[1], 10, 64)
	expires, _ := strconv.ParseInt(regexp.MustCompile(`;expires=(\d+)`).FindStringSubmatch(input)[1], 10, 64)
	if created != fixedNow.Unix() || expires-created != 60 {
		t.Fatalf("created %d, expires %d", created, expires)
	}

	base := "\"@authority\": heart-roasters.com\n\"signature-agent\": " + gotAgent + "\n\"@signature-params\": " + strings.TrimPrefix(input, "sig1=")
	value, err := base64.StdEncoding.DecodeString(regexp.MustCompile(`^sig1=:(.+):$`).FindStringSubmatch(sig)[1])
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := base64.RawURLEncoding.DecodeString(rfcPublicKey)
	if !ed25519.Verify(pub, []byte(base), value) {
		t.Fatal("the signature does not verify")
	}
	if got := r.Header.Get("User-Agent"); got != ua {
		t.Fatalf("User-Agent = %s", got)
	}
}

func TestAuthorityIsTheLowercaseHostWithANonDefaultPort(t *testing.T) {
	for raw, want := range map[string]string{
		"https://heart-roasters.com/products.json": "heart-roasters.com",
		"https://WWW.Example.COM/p":                "www.example.com",
		"https://shop.example:443/p":               "shop.example",
		"https://shop.example:8443/p":              "shop.example:8443",
	} {
		u, _ := url.Parse(raw)
		if got := Authority(u); got != want {
			t.Errorf("Authority(%s) = %s, want %s", raw, got, want)
		}
	}
}

func TestTheTransportSignsACloneAndLeavesTheCallersRequest(t *testing.T) {
	var sent *http.Request
	tr := &Transport{Base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent = r
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: r}, nil
	}), Signer: signer(t)}
	r, _ := http.NewRequest(http.MethodGet, "https://heart-roasters.com/products.json", nil)

	if _, err := tr.RoundTrip(r); err != nil {
		t.Fatal(err)
	}
	if sent.Header.Get("Signature") == "" {
		t.Fatal("the request sent was not signed")
	}
	if r.Header.Get("Signature") != "" {
		t.Fatal("the caller's request was changed")
	}
}

func TestNoKeyMeansNoSignerAndNoError(t *testing.T) {
	s, err := Parse("  ", agent, ua)
	if s != nil || err != nil {
		t.Fatalf("got %v, %v", s, err)
	}
}

func TestAnUnusableSettingIsAnError(t *testing.T) {
	good := base64.StdEncoding.EncodeToString(seed(t))
	for _, c := range []struct{ seed, agent, ua string }{
		{"not base64!", agent, ua},
		{"AAAA", agent, ua}, // 3 bytes
		{good, "http://api.wheyproof.com", ua},
		{good, "https://api.wheyproof.com/keys", ua},
		{good, agent, " "},
	} {
		if s, err := Parse(c.seed, c.agent, c.ua); s != nil || err == nil {
			t.Errorf("Parse(%q, %q, %q) = %v, %v; want an error", c.seed, c.agent, c.ua, s, err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
