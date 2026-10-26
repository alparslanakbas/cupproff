// Package webbotauth signs store requests with Web Bot Auth (HTTP Message
// Signatures, RFC 9421), so Shopify can tell our crawler apart from
// anonymous traffic.
//
// Why: Shopify holds bots that don't sign to its strictest storefront limits
// and gives registered, signing bots higher ones (its changelog, in force
// since 30 May 2026). The registration was made for the sister site's bot
// (wheyproofbot; forms of 8 and 18 Oct 2026). The key directory is served by
// WheyProof at https://api.wheyproof.com/.well-known/http-message-signatures-directory,
// so a request verifies only when it is signed with that same key.
//
// This is the signing half of WheyProof's WebBotAuth class (ProteinAvcısı
// has the same, as WebBotImzasi). The format details were pinned there by
// measurement: Signature-Agent is a QUOTED https URL (a dictionary member such
// as sig1="https://..." fails), keyid is the JWK thumbprint (RFC 8037 A.3),
// Ed25519 only. The three implementations share the RFC test vectors; a
// change here belongs in the others too.
package webbotauth

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequestValidity is how long a request signature stays valid.
const RequestValidity = time.Minute

// Signer adds the Web Bot Auth headers to a request.
type Signer struct {
	key       ed25519.PrivateKey
	agent     string // the Signature-Agent origin, unquoted
	userAgent string
	keyID     string
	now       func() time.Time
}

// New builds a signer from a 32-byte Ed25519 seed, the Signature-Agent origin
// (where the key directory is served) and the User-Agent signed requests
// carry (Shopify checks it against the registration).
func New(seed []byte, agent, userAgent string) (*Signer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("the key is %d bytes, not %d", len(seed), ed25519.SeedSize)
	}
	u, err := url.Parse(strings.TrimSpace(agent))
	if err != nil || u.Scheme != "https" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return nil, fmt.Errorf("signature agent %q is not an https origin", agent)
	}
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		return nil, errors.New("the user agent is empty")
	}
	key := ed25519.NewKeyFromSeed(seed)
	return &Signer{
		key:       key,
		agent:     "https://" + strings.ToLower(u.Host),
		userAgent: userAgent,
		keyID:     Thumbprint(base64URL(key.Public().(ed25519.PublicKey))),
		now:       time.Now,
	}, nil
}

// Parse builds a signer from a base64 seed, as kept in .env. An empty seed
// means no signing: it returns nil and no error.
func Parse(seedBase64, agent, userAgent string) (*Signer, error) {
	seedBase64 = strings.TrimSpace(seedBase64)
	if seedBase64 == "" {
		return nil, nil
	}
	seed, err := base64.StdEncoding.DecodeString(seedBase64)
	if err != nil {
		return nil, errors.New("the key is not base64")
	}
	return New(seed, agent, userAgent)
}

// KeyID is the key's JWK thumbprint, as named in every signature.
func (s *Signer) KeyID() string { return s.keyID }

// Agent is the Signature-Agent origin.
func (s *Signer) Agent() string { return s.agent }

// UserAgent is the User-Agent signed requests carry.
func (s *Signer) UserAgent() string { return s.userAgent }

// Sign adds Signature-Agent, Signature-Input and Signature to r and sets the
// bot's User-Agent. The signature covers the host and the agent only, so a
// retry of the same request may reuse it within RequestValidity.
func (s *Signer) Sign(r *http.Request) {
	created := s.now().Unix()
	agent := `"` + s.agent + `"`
	params := fmt.Sprintf(`("@authority" "signature-agent");created=%d;keyid="%s";alg="ed25519";expires=%d;nonce="%s";tag="web-bot-auth"`,
		created, s.keyID, created+int64(RequestValidity/time.Second), nonce())
	base := SignatureBase([][2]string{{"@authority", Authority(r.URL)}, {"signature-agent", agent}}, params)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(s.key, []byte(base)))

	r.Header.Set("Signature-Agent", agent)
	r.Header.Set("Signature-Input", "sig1="+params)
	r.Header.Set("Signature", "sig1=:"+sig+":")
	r.Header.Set("User-Agent", s.userAgent)
}

// Transport signs every request before handing it to Base. The request is
// cloned first: a RoundTripper must not change the caller's request.
type Transport struct {
	Base   http.RoundTripper
	Signer *Signer
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	signed := r.Clone(r.Context())
	t.Signer.Sign(signed)
	return t.Base.RoundTrip(signed)
}

// SignatureBase is RFC 9421 section 2.5: one line per covered component, then
// the parameters, no final newline.
func SignatureBase(components [][2]string, params string) string {
	var b strings.Builder
	for _, c := range components {
		fmt.Fprintf(&b, "\"%s\": %s\n", c[0], c[1])
	}
	b.WriteString(`"@signature-params": ` + params)
	return b.String()
}

// Authority is RFC 9421 section 2.2.3: the lowercase host, with the port only
// when it isn't the scheme's default.
func Authority(u *url.URL) string {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port == "" || (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		return host
	}
	return host + ":" + port
}

// Thumbprint is RFC 7638 over the required members in lexical order (RFC 8037
// A.3 for OKP keys); x is the public key, base64url.
func Thumbprint(x string) string {
	sum := sha256.Sum256([]byte(`{"crv":"Ed25519","kty":"OKP","x":"` + x + `"}`))
	return base64URL(sum[:])
}

func base64URL(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func nonce() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	return base64.StdEncoding.EncodeToString(b)
}
