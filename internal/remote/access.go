package remote

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// AccessHeader carries the token Cloudflare Access signs for every request
// it lets through to the tunnel.
const AccessHeader = "Cf-Access-Jwt-Assertion"

// Verifier checks a Cloudflare Access token for a team and application
// audience and returns the email it was issued to.
type Verifier interface {
	Verify(token, team, aud string) (string, error)
}

// AccessKeys verifies Access tokens against the team's public signing keys.
// The keys are fetched from https://<team>.cloudflareaccess.com, only when
// a token names a key not yet held, and at most once a minute. This is the
// only network call Agent Desk makes for remote access.
type AccessKeys struct {
	// Client fetches the keys. Nil uses a client with a 10 second timeout
	// that does not follow redirects.
	Client *http.Client
	// CertsURL builds the key URL; tests point it elsewhere.
	CertsURL func(team string) string
	// Now is the clock; nil is time.Now.
	Now func() time.Time

	mu      sync.Mutex
	team    string
	keys    map[string]*rsa.PublicKey
	lastTry time.Time
}

// refetchEvery bounds key fetches, so tokens naming unknown keys cannot
// make Agent Desk call Cloudflare in a loop.
const refetchEvery = time.Minute

// leeway allows for clock drift between Cloudflare and the desk.
const leeway = time.Minute

func (a *AccessKeys) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Issuer is the issuer Access puts in a team's tokens.
func Issuer(team string) string { return "https://" + team + ".cloudflareaccess.com" }

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtClaims struct {
	Iss   string          `json:"iss"`
	Aud   json.RawMessage `json:"aud"`
	Exp   int64           `json:"exp"`
	Nbf   int64           `json:"nbf"`
	Email string          `json:"email"`
}

// Verify checks the token's RS256 signature, issuer, audience, expiry and
// email.
func (a *AccessKeys) Verify(token, team, aud string) (string, error) {
	if !teamName.MatchString(team) || aud == "" {
		return "", errors.New("cloudflare access is not configured")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("malformed token")
	}
	var h jwtHeader
	if err := decodeSegment(parts[0], &h); err != nil {
		return "", err
	}
	if h.Alg != "RS256" || h.Kid == "" {
		return "", errors.New("unsupported token algorithm")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errors.New("malformed signature")
	}
	key, err := a.key(team, h.Kid)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return "", errors.New("bad signature")
	}
	var c jwtClaims
	if err := decodeSegment(parts[1], &c); err != nil {
		return "", err
	}
	if c.Iss != Issuer(team) {
		return "", errors.New("wrong issuer")
	}
	if !audienceHas(c.Aud, aud) {
		return "", errors.New("wrong audience")
	}
	now := a.now()
	if c.Exp == 0 || now.After(time.Unix(c.Exp, 0).Add(leeway)) {
		return "", errors.New("token expired")
	}
	if c.Nbf != 0 && now.Add(leeway).Before(time.Unix(c.Nbf, 0)) {
		return "", errors.New("token not yet valid")
	}
	email := strings.ToLower(strings.TrimSpace(c.Email))
	if email == "" {
		return "", errors.New("token has no email")
	}
	return email, nil
}

func decodeSegment(seg string, v any) error {
	data, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return errors.New("malformed token")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return errors.New("malformed token")
	}
	return nil
}

// audienceHas accepts aud as a string or a list of strings.
func audienceHas(raw json.RawMessage, aud string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == aud
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return slices.Contains(many, aud)
	}
	return false
}

// key returns the signing key with id kid, fetching the team's keys when
// it is not held and the last fetch was long enough ago.
func (a *AccessKeys) key(team, kid string) (*rsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.team != team {
		a.team, a.keys, a.lastTry = team, nil, time.Time{}
	}
	if k, ok := a.keys[kid]; ok {
		return k, nil
	}
	now := a.now()
	if !a.lastTry.IsZero() && now.Sub(a.lastTry) < refetchEvery {
		return nil, errors.New("unknown signing key")
	}
	a.lastTry = now
	keys, err := a.fetch(team)
	if err != nil {
		return nil, fmt.Errorf("fetching cloudflare access keys: %w", err)
	}
	a.keys = keys
	if k, ok := keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("unknown signing key")
}

type jwks struct {
	Keys []struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

func (a *AccessKeys) fetch(team string) (map[string]*rsa.PublicKey, error) {
	url := Issuer(team) + "/cdn-cgi/access/certs"
	if a.CertsURL != nil {
		url = a.CertsURL(team)
	}
	client := a.Client
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var set jwks
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		pub := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
		if pub.N.BitLen() < 2048 || pub.E < 3 {
			continue
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return nil, errors.New("no usable keys")
	}
	return keys, nil
}
