package server

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const googleIssuer = "https://accounts.google.com"
const googleKeysURL = "https://www.googleapis.com/oauth2/v3/certs"
const googleTokenURL = "https://oauth2.googleapis.com/token"
const googleAuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
const oauthStateCookie = "memona_google_state"
const userCookie = "memona_user_session"

type googleAttempt struct {
	verifier, nonce string
	expires         time.Time
}
type googleOIDC struct {
	clientID, clientSecret, redirectURI string
	client                              *http.Client
	mu                                  sync.Mutex
	attempts                            map[string]googleAttempt
}

func randomURLToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (a *App) googleStart(w http.ResponseWriter, r *http.Request) {
	if a.google == nil {
		fail(w, 404, "Login Google tidak tersedia.")
		return
	}
	if a.tunnelHost != "" && !strings.EqualFold(r.Host, a.tunnelHost) {
		fail(w, 403, "Login Google hanya tersedia melalui tunnel HTTPS.")
		return
	}
	if !a.security.loginPeers.Allow(directPeer(r), time.Now()) || !a.loginLimit.Allow() {
		fail(w, 429, "Terlalu banyak percobaan.")
		return
	}
	state, err := randomURLToken()
	if err != nil {
		a.internal(w, err)
		return
	}
	verifier, err := randomURLToken()
	if err != nil {
		a.internal(w, err)
		return
	}
	nonce, err := randomURLToken()
	if err != nil {
		a.internal(w, err)
		return
	}
	g := a.google
	g.mu.Lock()
	if g.attempts == nil {
		g.attempts = make(map[string]googleAttempt)
	}
	for key, attempt := range g.attempts {
		if time.Now().After(attempt.expires) {
			delete(g.attempts, key)
		}
	}
	if len(g.attempts) >= 512 {
		g.mu.Unlock()
		fail(w, 503, "Login sibuk.")
		return
	}
	g.attempts[state] = googleAttempt{verifier, nonce, time.Now().Add(5 * time.Minute)}
	g.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: state, Path: "/auth/google", HttpOnly: true, Secure: a.cfg.SecureCookies || a.tunnelHost != "", SameSite: http.SameSiteLaxMode, MaxAge: 300})
	challenge := sha256.Sum256([]byte(verifier))
	params := url.Values{"client_id": {g.clientID}, "redirect_uri": {g.redirectURI}, "response_type": {"code"}, "scope": {"openid email profile"}, "state": {state}, "nonce": {nonce}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}}
	http.Redirect(w, r, googleAuthURL+"?"+params.Encode(), http.StatusFound)
}

func (g *googleOIDC) consume(state string) (googleAttempt, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	attempt, ok := g.attempts[state]
	delete(g.attempts, state)
	return attempt, ok && time.Now().Before(attempt.expires)
}

// Only a callback with a live state and matching browser cookie may cross the
// fetch-site boundary. The state is consumed by googleCallback, not here.
func (a *App) googleCallbackNavigation(r *http.Request) bool {
	if a.google == nil || r.Method != http.MethodGet || r.URL.Path != "/auth/google/callback" || r.Header.Get("Origin") != "" {
		return false
	}
	c, err := r.Cookie(oauthStateCookie)
	if err != nil {
		return false
	}
	state := r.URL.Query().Get("state")
	if len(state) != 43 || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		return false
	}
	a.google.mu.Lock()
	attempt, exists := a.google.attempts[state]
	a.google.mu.Unlock()
	return exists && time.Now().Before(attempt.expires)
}

func (a *App) googleCallback(w http.ResponseWriter, r *http.Request) {
	if a.google == nil {
		fail(w, 404, "Login Google tidak tersedia.")
		return
	}
	if a.tunnelHost != "" && !strings.EqualFold(r.Host, a.tunnelHost) {
		fail(w, 403, "Login Google hanya tersedia melalui tunnel HTTPS.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/auth/google", HttpOnly: true, Secure: a.cfg.SecureCookies || a.tunnelHost != "", SameSite: http.SameSiteLaxMode, MaxAge: -1})
	c, err := r.Cookie(oauthStateCookie)
	state := r.URL.Query().Get("state")
	if err != nil || len(state) != 43 || subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) != 1 {
		fail(w, 400, "Status login tidak valid.")
		return
	}
	attempt, ok := a.google.consume(state)
	if !ok || r.URL.Query().Get("error") != "" {
		fail(w, 400, "Login kedaluwarsa atau dibatalkan.")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 4096 {
		fail(w, 400, "Kode login tidak valid.")
		return
	}
	claims, err := a.google.exchange(r.Context(), code, attempt)
	if err != nil {
		if r.Context().Err() != nil {
			readFailure(w, r.Context().Err())
		} else {
			fail(w, http.StatusUnauthorized, "Verifikasi login Google gagal.")
		}
		return
	}
	user, token, _, err := a.auth.UserLoginContext(r.Context(), claims.Sub, claims.Email, claims.Name)
	if err != nil {
		a.internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: userCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookies || a.tunnelHost != "", SameSite: http.SameSiteLaxMode, MaxAge: 8 * 60 * 60})
	a.audit("user:"+user.ID, "login", "")
	http.Redirect(w, r, "/user", http.StatusSeeOther)
}

func (a *App) userSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(userCookie)
	if err != nil {
		fail(w, 401, "Login diperlukan.")
		return
	}
	u, csrf, err := a.auth.UserSessionContext(r.Context(), c.Value)
	if err != nil {
		if readFailure(w, err) {
			return
		}
		fail(w, 401, "Sesi berakhir.")
		return
	}
	respond(w, 200, map[string]any{"user": u, "csrf": csrf})
}

func (a *App) userLogout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(userCookie)
	if err != nil {
		fail(w, 401, "Login diperlukan.")
		return
	}
	u, csrf, err := a.auth.UserSessionContext(r.Context(), c.Value)
	if err != nil {
		if readFailure(w, err) {
			return
		}
		fail(w, 401, "Sesi berakhir.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(csrf), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
		fail(w, 403, "Token CSRF tidak valid.")
		return
	}
	if err := a.auth.UserLogoutContext(r.Context(), c.Value); err != nil {
		a.internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: userCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookies || a.tunnelHost != "", SameSite: http.SameSiteLaxMode, MaxAge: -1})
	a.audit("user:"+u.ID, "logout", "")
	respond(w, 200, map[string]bool{"ok": true})
}

type googleClaims struct {
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	Nonce         string `json:"nonce"`
	EmailVerified bool   `json:"email_verified"`
	Exp           int64  `json:"exp"`
	Iat           int64  `json:"iat"`
}

func (g *googleOIDC) exchange(ctx context.Context, code string, attempt googleAttempt) (googleClaims, error) {
	form := url.Values{"code": {code}, "client_id": {g.clientID}, "client_secret": {g.clientSecret}, "redirect_uri": {g.redirectURI}, "grant_type": {"authorization_code"}, "code_verifier": {attempt.verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleClaims{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.client.Do(req)
	if err != nil {
		return googleClaims{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleClaims{}, errors.New("Google token exchange failed")
	}
	var result struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&result); err != nil {
		return googleClaims{}, err
	}
	return g.verifyIDToken(ctx, result.IDToken, attempt.nonce)
}

func (g *googleOIDC) verifyIDToken(ctx context.Context, token, nonce string) (googleClaims, error) {
	bad := errors.New("invalid Google ID token")
	if len(token) > 16<<10 {
		return googleClaims{}, bad
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return googleClaims{}, bad
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return googleClaims{}, bad
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if json.Unmarshal(headerBytes, &header) != nil || header.Alg != "RS256" || header.Kid == "" {
		return googleClaims{}, bad
	}
	signed := []byte(parts[0] + "." + parts[1])
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return googleClaims{}, bad
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleKeysURL, nil)
	if err != nil {
		return googleClaims{}, err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return googleClaims{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleClaims{}, errors.New("Google JWKS unavailable")
	}
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&jwks); err != nil {
		return googleClaims{}, err
	}
	verified := false
	for _, key := range jwks.Keys {
		if key.Kid != header.Kid || key.Kty != "RSA" || (key.Alg != "" && key.Alg != "RS256") || (key.Use != "" && key.Use != "sig") {
			continue
		}
		n, e1 := base64.RawURLEncoding.DecodeString(key.N)
		e, e2 := base64.RawURLEncoding.DecodeString(key.E)
		if e1 != nil || e2 != nil || len(n) < 256 || len(n) > 512 || len(e) == 0 || len(e) > 4 {
			continue
		}
		exponent := new(big.Int).SetBytes(e)
		if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > 65537 || exponent.Int64()%2 == 0 {
			continue
		}
		h := sha256.Sum256(signed)
		if rsa.VerifyPKCS1v15(&rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())}, crypto.SHA256, h[:], sig) == nil {
			verified = true
			break
		}
	}
	if !verified {
		return googleClaims{}, bad
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return googleClaims{}, bad
	}
	var claims googleClaims
	if json.Unmarshal(body, &claims) != nil {
		return googleClaims{}, bad
	}
	now := time.Now().Unix()
	if (claims.Iss != googleIssuer && claims.Iss != "accounts.google.com") || claims.Aud != g.clientID || claims.Exp <= now || claims.Iat > now+60 || claims.Iat <= 0 || claims.Exp-claims.Iat > 86400 || claims.Nonce != nonce || nonce == "" || claims.Sub == "" || len(claims.Sub) > 255 || !claims.EmailVerified || claims.Email == "" || len(claims.Email) > 320 {
		return googleClaims{}, bad
	}
	return claims, nil
}
