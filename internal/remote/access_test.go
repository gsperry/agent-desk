package remote

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testAUD = "0123456789abcdef0123456789abcdef"

var testNow = time.Unix(1_800_000_000, 0)

type keyServer struct {
	srv   *httptest.Server
	hits  atomic.Int32
	keys  map[string]*rsa.PrivateKey
	extra []map[string]string
}

func newKeyServer(t *testing.T, kids ...string) *keyServer {
	t.Helper()
	ks := &keyServer{keys: map[string]*rsa.PrivateKey{}}
	for _, kid := range kids {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		ks.keys[kid] = k
	}
	ks.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ks.hits.Add(1)
		if r.URL.Path != "/cdn-cgi/access/certs" {
			http.NotFound(w, r)
			return
		}
		set := []map[string]string{}
		for kid, k := range ks.keys {
			set = append(set, map[string]string{
				"kid": kid, "kty": "RSA", "alg": "RS256",
				"n": b64(k.N.Bytes()), "e": b64(big.NewInt(int64(k.E)).Bytes()),
			})
		}
		set = append(set, ks.extra...)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": set})
	}))
	t.Cleanup(ks.srv.Close)
	return ks
}

func (ks *keyServer) verifier() *AccessKeys {
	return &AccessKeys{
		Client:   ks.srv.Client(),
		CertsURL: func(string) string { return ks.srv.URL + "/cdn-cgi/access/certs" },
		Now:      func() time.Time { return testNow },
	}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func sign(t *testing.T, k *rsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(header)
	c, _ := json.Marshal(claims)
	input := b64(h) + "." + b64(c)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + b64(sig)
}

func goodClaims() map[string]any {
	return map[string]any{
		"iss":   Issuer("team"),
		"aud":   []string{testAUD},
		"exp":   testNow.Add(time.Hour).Unix(),
		"nbf":   testNow.Add(-time.Minute).Unix(),
		"email": "Dev@Example.com",
		"type":  "app",
	}
}

func TestAccessVerify(t *testing.T) {
	ks := newKeyServer(t, "k1")
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	hdr := map[string]any{"alg": "RS256", "kid": "k1"}
	with := func(key string, v any) map[string]any {
		c := goodClaims()
		if v == nil {
			delete(c, key)
		} else {
			c[key] = v
		}
		return c
	}
	tests := []struct {
		name  string
		token string
		team  string
		ok    bool
	}{
		{"valid", sign(t, ks.keys["k1"], hdr, goodClaims()), "team", true},
		{"audience as a string", sign(t, ks.keys["k1"], hdr, with("aud", testAUD)), "team", true},
		{"within leeway", sign(t, ks.keys["k1"], hdr, with("exp", testNow.Add(-30*time.Second).Unix())), "team", true},
		{"expired", sign(t, ks.keys["k1"], hdr, with("exp", testNow.Add(-2*time.Minute).Unix())), "team", false},
		{"no expiry", sign(t, ks.keys["k1"], hdr, with("exp", nil)), "team", false},
		{"not yet valid", sign(t, ks.keys["k1"], hdr, with("nbf", testNow.Add(5*time.Minute).Unix())), "team", false},
		{"wrong audience", sign(t, ks.keys["k1"], hdr, with("aud", []string{"another"})), "team", false},
		{"wrong issuer", sign(t, ks.keys["k1"], hdr, with("iss", Issuer("other"))), "team", false},
		{"other team configured", sign(t, ks.keys["k1"], hdr, goodClaims()), "other", false},
		{"no email", sign(t, ks.keys["k1"], hdr, with("email", nil)), "team", false},
		{"signed by another key", sign(t, other, hdr, goodClaims()), "team", false},
		{"unknown key id", sign(t, ks.keys["k1"], map[string]any{"alg": "RS256", "kid": "nope"}, goodClaims()), "team", false},
		{"alg none", b64([]byte(`{"alg":"none","kid":"k1"}`)) + "." + b64(mustJSON(goodClaims())) + ".", "team", false},
		{"HS256", sign(t, ks.keys["k1"], map[string]any{"alg": "HS256", "kid": "k1"}, goodClaims()), "team", false},
		{"two parts", "a.b", "team", false},
		{"garbage", "!!!.???.***", "team", false},
		{"bad team name", sign(t, ks.keys["k1"], hdr, goodClaims()), "evil.com/x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := ks.verifier().Verify(tt.token, tt.team, testAUD)
			if (err == nil) != tt.ok {
				t.Fatalf("err %v, want ok %v", err, tt.ok)
			}
			if tt.ok && email != "dev@example.com" {
				t.Fatalf("email %q", email)
			}
		})
	}
}

func TestTamperedPayload(t *testing.T) {
	ks := newKeyServer(t, "k1")
	tok := sign(t, ks.keys["k1"], map[string]any{"alg": "RS256", "kid": "k1"}, goodClaims())
	parts := strings.Split(tok, ".")
	c := goodClaims()
	c["email"] = "eve@example.com"
	parts[1] = b64(mustJSON(c))
	if _, err := ks.verifier().Verify(strings.Join(parts, "."), "team", testAUD); err == nil {
		t.Fatal("tampered token verified")
	}
}

func TestAccessKeyFetching(t *testing.T) {
	ks := newKeyServer(t, "k1")
	v := ks.verifier()
	now := testNow
	v.Now = func() time.Time { return now }
	good := sign(t, ks.keys["k1"], map[string]any{"alg": "RS256", "kid": "k1"}, goodClaims())
	for range 3 {
		if _, err := v.Verify(good, "team", testAUD); err != nil {
			t.Fatal(err)
		}
	}
	if n := ks.hits.Load(); n != 1 {
		t.Fatalf("held keys refetched: %d fetches", n)
	}
	// Unknown key ids refetch at most once a minute.
	unknown := sign(t, ks.keys["k1"], map[string]any{"alg": "RS256", "kid": "k2"}, goodClaims())
	for range 5 {
		_, _ = v.Verify(unknown, "team", testAUD)
	}
	if n := ks.hits.Load(); n != 1 {
		t.Fatalf("unknown key fetched within a minute: %d fetches", n)
	}
	// After a minute, a rotated key is picked up.
	now = now.Add(2 * time.Minute)
	k2, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks.keys["k2"] = k2
	claims := goodClaims()
	claims["exp"] = now.Add(time.Hour).Unix()
	rotated := sign(t, k2, map[string]any{"alg": "RS256", "kid": "k2"}, claims)
	if _, err := v.Verify(rotated, "team", testAUD); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
	if n := ks.hits.Load(); n != 2 {
		t.Fatalf("%d fetches", n)
	}
}

func TestAccessKeysSkipsWeakKeys(t *testing.T) {
	ks := newKeyServer(t)
	weak, _ := rsa.GenerateKey(rand.Reader, 1024)
	ks.extra = []map[string]string{
		{"kid": "weak", "kty": "RSA", "n": b64(weak.N.Bytes()), "e": b64(big.NewInt(int64(weak.E)).Bytes())},
		{"kid": "ec", "kty": "EC"},
	}
	tok := sign(t, weak, map[string]any{"alg": "RS256", "kid": "weak"}, goodClaims())
	if _, err := ks.verifier().Verify(tok, "team", testAUD); err == nil {
		t.Fatal("1024-bit key accepted")
	}
}

func TestAccessKeysDoNotFollowRedirects(t *testing.T) {
	target := newKeyServer(t, "k1")
	redirect := httptest.NewServer(http.RedirectHandler(target.srv.URL+"/cdn-cgi/access/certs", http.StatusFound))
	t.Cleanup(redirect.Close)
	v := &AccessKeys{
		CertsURL: func(string) string { return redirect.URL },
		Now:      func() time.Time { return testNow },
	}
	tok := sign(t, target.keys["k1"], map[string]any{"alg": "RS256", "kid": "k1"}, goodClaims())
	if _, err := v.Verify(tok, "team", testAUD); err == nil {
		t.Fatal("followed a redirect to another key set")
	}
	if target.hits.Load() != 0 {
		t.Fatal("redirect target was fetched")
	}
}

type fakeVerifier struct{ email string }

func (f fakeVerifier) Verify(token, team, aud string) (string, error) {
	if token != "good" || team != "team" || aud != testAUD {
		return "", errBad
	}
	return f.email, nil
}

var errBad = errors.New("bad token")

func TestIdentifyCloudflare(t *testing.T) {
	s := Settings{Via: ViaCloudflare, CloudflareTeam: "team", CloudflareAUD: testAUD, CloudflareEmails: []string{"dev@example.com"}, TailscaleLogins: []string{"dev@example.com"}}
	tests := []struct {
		name    string
		headers map[string][]string
		v       Verifier
		want    bool
	}{
		{"valid token", map[string][]string{AccessHeader: {"good"}}, fakeVerifier{"dev@example.com"}, true},
		{"email not allowed", map[string][]string{AccessHeader: {"good"}}, fakeVerifier{"eve@example.com"}, false},
		{"bad token", map[string][]string{AccessHeader: {"bad"}}, fakeVerifier{"dev@example.com"}, false},
		{"two tokens", map[string][]string{AccessHeader: {"good", "good"}}, fakeVerifier{"dev@example.com"}, false},
		{"no verifier", map[string][]string{AccessHeader: {"good"}}, nil, false},
		{"tailscale header ignored", map[string][]string{TailscaleHeader: {"dev@example.com"}}, fakeVerifier{"dev@example.com"}, false},
		{"email header alone ignored", map[string][]string{"Cf-Access-Authenticated-User-Email": {"dev@example.com"}}, fakeVerifier{"dev@example.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			for k, vs := range tt.headers {
				for _, v := range vs {
					r.Header.Add(k, v)
				}
			}
			id, ok := Identify(r, s, tt.v)
			if ok != tt.want || (ok && (id.User != "dev@example.com" || id.Via != ViaCloudflare)) {
				t.Fatalf("got %+v %v", id, ok)
			}
		})
	}
	// Under Tailscale, an Access token counts for nothing.
	ts := s
	ts.Via = ViaTailscale
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(AccessHeader, "good")
	if _, ok := Identify(r, ts, fakeVerifier{"dev@example.com"}); ok {
		t.Fatal("access token accepted under tailscale")
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
