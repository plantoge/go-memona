package server

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGoogleTokenChecks(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding
	n := enc.EncodeToString(key.N.Bytes())
	e := enc.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	keys, _ := json.Marshal(map[string]any{"keys": []any{map[string]string{"kid": "key1", "kty": "RSA", "alg": "RS256", "n": n, "e": e}}})
	g := googleOIDC{clientID: "client1", client: &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: ioNopCloser{strings.NewReader(string(keys))}, Header: make(http.Header)}, nil
	})}}
	base := googleClaims{Iss: googleIssuer, Aud: "client1", Sub: "subject", Email: "person@example.org", EmailVerified: true, Nonce: "nonce", Iat: time.Now().Unix(), Exp: time.Now().Add(time.Hour).Unix()}
	sign := func(c googleClaims) string {
		t.Helper()
		h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "key1"})
		b, _ := json.Marshal(c)
		prefix := enc.EncodeToString(h) + "." + enc.EncodeToString(b)
		hash := sha256.Sum256([]byte(prefix))
		sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if err != nil {
			t.Fatal(err)
		}
		return prefix + "." + enc.EncodeToString(sig)
	}
	if _, err = g.verifyIDToken(context.Background(), sign(base), "nonce"); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*googleClaims)
		nonce  string
	}{
		{"nonce", func(c *googleClaims) {}, "wrong"},
		{"issuer", func(c *googleClaims) { c.Iss = "https://evil.test" }, "nonce"},
		{"audience", func(c *googleClaims) { c.Aud = "other" }, "nonce"},
		{"expired", func(c *googleClaims) { c.Exp = time.Now().Add(-time.Minute).Unix() }, "nonce"},
		{"email", func(c *googleClaims) { c.EmailVerified = false }, "nonce"},
		{"subject", func(c *googleClaims) { c.Sub = "" }, "nonce"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.change(&c)
			if _, err := g.verifyIDToken(context.Background(), sign(c), tc.nonce); err == nil {
				t.Fatal("accepted invalid token")
			}
		})
	}
	tok := sign(base)
	parts := strings.Split(tok, ".")
	parts[1] = enc.EncodeToString([]byte(`{"sub":"attacker"}`))
	if _, err := g.verifyIDToken(context.Background(), strings.Join(parts, "."), "nonce"); err == nil {
		t.Fatal("accepted tampered signature")
	}
}

type ioNopCloser struct{ *strings.Reader }

func (ioNopCloser) Close() error { return nil }

func TestGoogleStateSingleUse(t *testing.T) {
	g := &googleOIDC{attempts: map[string]googleAttempt{"state": {verifier: "v", nonce: "n", expires: time.Now().Add(time.Minute)}}}
	if _, ok := g.consume("state"); !ok {
		t.Fatal("initial state invalid")
	}
	if _, ok := g.consume("state"); ok {
		t.Fatal("replayed state")
	}
}

func TestGoogleLoginWithPrivateHTTPAndTunnel(t *testing.T) {
	f := setup(t)
	a, err := New(Config{Listen: "192.168.1.5:8765", PublicURL: "http://192.168.1.5:8765", TunnelURL: "https://memona.example.com", AllowPrivateHTTP: true, GoogleClientID: "example", GoogleClientSecret: "synthetic"}, f.auth, f.notes)
	if err != nil {
		t.Fatal(err)
	}
	if a.google.redirectURI != "https://memona.example.com/auth/google/callback" {
		t.Fatalf("wrong callback URI: %s", a.google.redirectURI)
	}
	request := func(host, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://192.168.1.5:8765"+path, nil)
		r.Host = host
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	if w := request("192.168.1.5:8765", "/auth/google/start"); w.Code != 403 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("private HTTP Google start: %d", w.Code)
	}
	w := request("memona.example.com", "/auth/google/start")
	if w.Code != http.StatusFound {
		t.Fatalf("tunnel Google start: %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("Google state cookie is not secure")
	}
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || location.Query().Get("redirect_uri") != a.google.redirectURI {
		t.Fatalf("wrong OAuth redirect URI: %v", err)
	}
	if w := request("192.168.1.5:8765", "/auth/google/callback"); w.Code != 403 {
		t.Fatalf("private HTTP Google callback: %d", w.Code)
	}
}
