package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yuin/goldmark"
	"golang.org/x/time/rate"
	"memona/internal/access"
	"memona/internal/memory"
)

type Config struct {
	Listen             string `json:"listen"`
	DataDir            string `json:"data_dir"`
	PublicURL          string `json:"public_url"`
	TunnelURL          string `json:"tunnel_url"`
	SecureCookies      bool   `json:"secure_cookies"`
	AllowPrivateHTTP   bool   `json:"allow_private_http"`
	GoogleClientID     string `json:"google_client_id,omitempty"`
	GoogleClientSecret string `json:"google_client_secret,omitempty"`
}
type App struct {
	cfg          Config
	origin       string
	host         string
	tunnelHost   string
	tunnelOrigin string
	auth         *access.Store
	notes        *memory.Store
	loginLimit   *rate.Limiter
	mcpLimit     *rate.Limiter
	handler      http.Handler
	workSlots    chan struct{}
	keyLimits    *keyLimiter
	security     *requestSecurity
	google       *googleOIDC
}
type authenticatedKeyContext struct{}

// Private HTTP is an explicit development/tailnet option, not public TLS.
func privateIP(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost"
	}
	_, tailnet, _ := net.ParseCIDR("100.64.0.0/10")
	return ip.IsLoopback() || ip.IsPrivate() || tailnet.Contains(ip)
}

// Memona serves HTTP directly or to a TLS reverse proxy.
func ValidateConfig(cfg Config) error {
	if (cfg.GoogleClientID == "") != (cfg.GoogleClientSecret == "") {
		return errors.New("Google login requires both google_client_id and google_client_secret")
	}
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return errors.New("listen must be an explicit IP:port, such as 0.0.0.0:8765")
	}
	ip := net.ParseIP(host)
	number, err := strconv.Atoi(port)
	if ip == nil || err != nil || number < 1 || number > 65535 {
		return errors.New("listen requires a valid IP and port 1–65535")
	}
	u, err := url.Parse(cfg.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("public_url must be an HTTP(S) origin without path or credentials")
	}
	if address := net.ParseIP(u.Hostname()); address != nil && address.IsUnspecified() {
		return errors.New("public_url must be a reachable hostname or IP, not a wildcard bind address")
	}
	if u.Scheme == "https" && !cfg.SecureCookies {
		return errors.New("HTTPS public_url requires secure_cookies=true")
	}
	if cfg.AllowPrivateHTTP {
		if u.Scheme != "http" || !privateIP(u.Hostname()) || cfg.SecureCookies {
			return errors.New("allow_private_http requires a private/loopback/Tailscale HTTP URL and secure_cookies=false")
		}
		_, listenPort, _ := net.SplitHostPort(cfg.Listen)
		if u.Port() != listenPort {
			return errors.New("private HTTP public_url port must match listen port")
		}
	} else {
		if !ip.IsLoopback() && u.Scheme != "https" {
			return errors.New("network listen requires HTTPS or explicit allow_private_http")
		}
		if u.Scheme == "http" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1" {
			return errors.New("public HTTP is not supported; use HTTPS")
		}
	}
	if cfg.TunnelURL != "" {
		if !cfg.AllowPrivateHTTP {
			return errors.New("tunnel_url requires allow_private_http=true")
		}
		tunnel, err := url.Parse(cfg.TunnelURL)
		if err != nil || tunnel.Scheme != "https" || tunnel.Host == "" || tunnel.User != nil || tunnel.RawQuery != "" || tunnel.Fragment != "" || (tunnel.Path != "" && tunnel.Path != "/") || tunnel.Port() != "" || net.ParseIP(tunnel.Hostname()) != nil {
			return errors.New("tunnel_url must be an HTTPS hostname without port, path or credentials")
		}
	}
	if cfg.GoogleClientID != "" && u.Scheme != "https" && cfg.TunnelURL == "" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") && !cfg.AllowPrivateHTTP) {
		return errors.New("Google login requires HTTPS public_url or tunnel_url except loopback development")
	}
	return nil
}

func New(cfg Config, auth *access.Store, notes *memory.Store) (*App, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	u, _ := url.Parse(cfg.PublicURL)
	a := &App{cfg: cfg, origin: u.Scheme + "://" + u.Host, host: u.Host, auth: auth, notes: notes, loginLimit: rate.NewLimiter(rate.Every(12*time.Second), 5), mcpLimit: rate.NewLimiter(30, 60), workSlots: make(chan struct{}, 8)}
	if cfg.TunnelURL != "" {
		tunnel, _ := url.Parse(cfg.TunnelURL)
		a.tunnelHost = tunnel.Host
		a.tunnelOrigin = "https://" + tunnel.Host
	}
	a.keyLimits = newKeyLimiter(4096, 10, 20, 15*time.Minute)
	a.security = newRequestSecurity()
	if cfg.GoogleClientID != "" {
		googleOrigin := a.origin
		if a.tunnelHost != "" {
			googleOrigin = a.tunnelOrigin
		}
		a.google = &googleOIDC{clientID: cfg.GoogleClientID, clientSecret: cfg.GoogleClientSecret, redirectURI: googleOrigin + "/auth/google/callback", client: &http.Client{Timeout: 10 * time.Second}}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /auth/google/start", a.googleStart)
	mux.HandleFunc("GET /auth/google/callback", a.googleCallback)
	mux.HandleFunc("GET /api/user/session", a.userSession)
	mux.HandleFunc("POST /api/user/logout", a.userLogout)
	mux.HandleFunc("/api/user/", a.userAPI)
	mux.HandleFunc("POST /api/login", a.login)
	mux.Handle("/api/", a.admin(http.HandlerFunc(a.api)))
	mh := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		k, ok := r.Context().Value(authenticatedKeyContext{}).(access.Key)
		if !ok {
			return nil
		}
		return a.mcpServerContext(r.Context(), k)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true, MaxRequestBodyBytes: 128 << 10})
	// SDK localhost protection is replaced by an explicit configured Host allowlist
	// below, so a legitimate tunnel Host is accepted without allowing arbitrary hosts.
	mux.Handle("/mcp", a.secureMCP(mh))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/", dashboardAsset)
	a.handler = a.boundary(mux)
	return a, nil
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.handler.ServeHTTP(w, r) }
func (a *App) boundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		isTunnel := a.tunnelHost != "" && strings.EqualFold(r.Host, a.tunnelHost)
		if a.cfg.SecureCookies || isTunnel {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		expectedOrigin := a.origin
		allowedHost := strings.EqualFold(r.Host, a.host)
		if isTunnel {
			allowedHost = true
			expectedOrigin = a.tunnelOrigin
		} else if a.cfg.AllowPrivateHTTP {
			host, port, err := net.SplitHostPort(r.Host)
			_, listenPort, _ := net.SplitHostPort(a.cfg.Listen)
			allowedHost = err == nil && port == listenPort && privateIP(host)
			expectedOrigin = "http://" + r.Host
		}
		if !allowedHost {
			fail(w, 403, "host not allowed")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != expectedOrigin {
			fail(w, 403, "origin not allowed")
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" && !a.googleCallbackNavigation(r) {
			fail(w, 403, "cross-site request denied")
			return
		}
		select {
		case a.workSlots <- struct{}{}:
			defer func() { <-a.workSlots }()
		default:
			fail(w, 503, "server busy; retry later")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/mcp" {
			ctx, cancel := context.WithTimeout(r.Context(), memory.ReadTimeout)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	data, err := encodeBoundedJSON(v)
	if err != nil {
		status = http.StatusServiceUnavailable
		data = []byte("{\"error\":\"Respons melampaui batas atau gagal dienkode; persempit hasil.\"}\n")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
func fail(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func decode(r *http.Request, v any) error {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		return errors.New("application/json required")
	}
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return e
	}
	var rest any
	if e := d.Decode(&rest); e != io.EOF {
		return errors.New("one JSON object required")
	}
	return nil
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !a.security.loginPeers.Allow(directPeer(r), time.Now()) || !a.loginLimit.Allow() {
		a.security.limited.Add(1)
		w.Header().Set("Retry-After", "12")
		fail(w, 429, "Terlalu banyak percobaan. Tunggu lalu coba lagi.")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if decode(r, &in) != nil {
		fail(w, 400, "Permintaan login tidak valid.")
		return
	}
	if !a.reserveAuthentication(w) {
		return
	}
	token, csrf, e := func() (string, string, error) {
		defer func() { <-a.security.authSlots }()
		return a.auth.LoginContext(r.Context(), in.Password)
	}()
	if e != nil {
		if errors.Is(e, access.ErrDenied) {
			a.authenticationFailed(r.Context())
		}
		if readFailure(w, e) {
			return
		}
		fail(w, 401, "Login gagal.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "memona_session", Value: token, Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookies || (a.tunnelHost != "" && strings.EqualFold(r.Host, a.tunnelHost)), SameSite: http.SameSiteStrictMode, MaxAge: 8 * 60 * 60})
	a.audit("admin", "login", "")
	respond(w, 200, map[string]string{"csrf": csrf})
}
func (a *App) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, e := r.Cookie("memona_session")
		if e != nil {
			fail(w, 401, "Login diperlukan.")
			return
		}
		csrf, e := a.auth.SessionContext(r.Context(), cookie.Value)
		if e != nil {
			if readFailure(w, e) {
				return
			}
			fail(w, 401, "Sesi berakhir. Silakan login kembali.")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && subtle.ConstantTimeCompare([]byte(csrf), []byte(r.Header.Get("X-CSRF-Token"))) != 1 {
			fail(w, 403, "Token CSRF tidak valid.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) api(w http.ResponseWriter, r *http.Request) {
	if a.editorAPI(w, r) || a.projectsAPI(w, r) || a.duplicatesAPI(w, r) || a.historyPageAPI(w, r) || a.skillRevisionAPI(w, r) {
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/api/graph" {
		a.graphAPI(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case path == "session" && r.Method == "GET":
		c, _ := r.Cookie("memona_session")
		csrf, err := a.auth.SessionContext(r.Context(), c.Value)
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]string{"csrf": csrf})
	case path == "logout" && r.Method == "POST":
		c, _ := r.Cookie("memona_session")
		if e := a.auth.LogoutContext(r.Context(), c.Value); e != nil {
			a.internal(w, e)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "memona_session", Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.SecureCookies || (a.tunnelHost != "" && strings.EqualFold(r.Host, a.tunnelHost)), SameSite: http.SameSiteStrictMode, MaxAge: -1})
		respond(w, 200, map[string]bool{"ok": true})
	case path == "server-info" && r.Method == "GET":
		publicURL := a.cfg.PublicURL
		if a.tunnelHost != "" && strings.EqualFold(r.Host, a.tunnelHost) {
			publicURL = a.cfg.TunnelURL
		}
		respond(w, 200, map[string]string{
			"mcp_endpoint": strings.TrimRight(publicURL, "/") + "/mcp",
			"transport":    "Streamable HTTP",
		})
	case path == "request-security" && r.Method == "GET":
		a.requestSecurityAPI(w)
	case path == "storage-health" && r.Method == "GET":
		report, err := a.notes.Health(r.Context())
		if errors.Is(err, memory.ErrInspectBusy) {
			w.Header().Set("Retry-After", "5")
			fail(w, 503, "Penyimpanan sibuk. Coba lagi setelah beberapa saat.")
			return
		}
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, report)
	case path == "keys-page" && r.Method == "GET":
		a.keyPageAPI(w, r)
	case path == "keys" && r.Method == "GET":
		v, e := a.auth.KeysContext(r.Context())
		if e != nil {
			a.internal(w, e)
			return
		}
		respond(w, 200, v)
	case path == "key-usage" && r.Method == "GET":
		keys, err := a.auth.KeysContext(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		events, err := a.auth.ActivityContext(r.Context())
		if err != nil {
			a.internal(w, err)
			return
		}
		respond(w, 200, map[string]any{"usage": recordedKeyUsage(keys, events), "audit_window": 200})
	case path == "keys" && r.Method == "POST":
		var in access.KeyInput
		if decode(r, &in) != nil {
			fail(w, 400, "Pengaturan key tidak valid.")
			return
		}
		k, token, e := a.auth.CreateKeyContext(r.Context(), in)
		if e != nil {
			if errors.Is(e, access.ErrInvalid) {
				fail(w, 400, "Nama, izin, proyek, atau kedaluwarsa tidak valid.")
			} else {
				a.internal(w, e)
			}
			return
		}
		respond(w, 201, map[string]any{"key": k, "token": token})
	case strings.HasPrefix(path, "keys/") && strings.HasSuffix(path, "/revoke") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "keys/"), "/revoke")
		if e := a.auth.RevokeContext(r.Context(), id); e != nil {
			fail(w, 400, "Key tidak ditemukan atau tidak dapat dicabut.")
			return
		}
		respond(w, 200, map[string]bool{"ok": true})
	case path == "activity" && r.Method == "GET":
		v, e := a.auth.ActivityContext(r.Context())
		if e != nil {
			a.internal(w, e)
			return
		}
		respond(w, 200, v)
	case path == "projects" && r.Method == "GET":
		notes, e := a.notes.ListContext(r.Context())
		if e != nil {
			a.internal(w, e)
			return
		}
		set := map[string]bool{"global": true}
		for _, n := range notes {
			if readFailure(w, r.Context().Err()) {
				return
			}
			set[n.Project] = true
		}
		out := []string{}
		for p := range set {
			out = append(out, p)
		}
		sort.Strings(out)
		respond(w, 200, out)
	case path == "notes" && r.Method == "GET":
		notes, e := a.notes.ListContext(r.Context())
		if e != nil {
			a.internal(w, e)
			return
		}
		out := []noteView{}
		q := strings.ToLower(r.URL.Query().Get("q"))
		if !validRetrievalText(q, 2000, true, false) {
			a.noteError(w, memory.ErrInvalid)
			return
		}
		project := r.URL.Query().Get("project")
		kind := r.URL.Query().Get("kind")
		for _, n := range notes {
			if readFailure(w, r.Context().Err()) {
				return
			}
			if (project == "" || n.Project == project) && (kind == "" || n.Kind == kind) {
				out = append(out, noteView{Note: n})
			}
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].IsProjectSummary != out[j].IsProjectSummary {
				return out[i].IsProjectSummary
			}
			if out[i].UpdatedAt == out[j].UpdatedAt {
				return out[i].ID < out[j].ID
			}
			return out[i].UpdatedAt > out[j].UpdatedAt
		})
		if q != "" {
			// Admin listing intentionally includes archived notes. Rank copies with
			// only that flag cleared, and return the original flags unchanged.
			candidates := make([]memory.Note, 0, len(out))
			originals := make(map[string]memory.Note, len(out))
			for _, view := range out {
				originals[view.ID] = view.Note
				n := view.Note
				n.Archived = false
				candidates = append(candidates, n)
			}
			adminKey := access.Key{Permissions: []string{"read"}, Projects: []string{"*"}}
			ranked, err := querySnapshot(r.Context(), adminKey, queryInput{Query: q}, candidates)
			if err != nil {
				if !readFailure(w, err) {
					a.noteError(w, err)
				}
				return
			}
			out = []noteView{}
			for _, n := range ranked {
				out = append(out, noteView{Note: originals[n.ID]})
			}
		}
		if len(out) > 500 {
			out = out[:500]
		}
		for i := range out {
			links, err := connectionsForContext(r.Context(), out[i].Note, notes)
			if err != nil {
				if !readFailure(w, err) {
					a.internal(w, err)
				}
				return
			}
			out[i].Connections = links
		}
		respond(w, 200, out)
	case strings.HasPrefix(path, "notes/"):
		parts := strings.Split(strings.TrimPrefix(path, "notes/"), "/")
		id := parts[0]
		if len(parts) == 1 && r.Method == "GET" {
			n, e := a.notes.GetContext(r.Context(), id)
			if e != nil {
				a.noteError(w, e)
				return
			}
			var history []memory.Note
			var nextBefore int
			if r.URL.Query().Get("history") == "paged" {
				var page memory.HistoryPage
				page, e = a.notes.HistoryPageContext(r.Context(), id, 0, 20)
				history = page.Items
				nextBefore = page.Before
			} else {
				history, e = a.notes.HistoryContext(r.Context(), id)
			}
			if e != nil {
				a.internal(w, e)
				return
			}
			var b bytes.Buffer
			if e = goldmark.Convert([]byte(n.Content), &b); e != nil {
				a.internal(w, e)
				return
			}
			all, err := a.notes.ListContext(r.Context())
			if err != nil {
				a.internal(w, err)
				return
			}
			links, err := connectionsForContext(r.Context(), n, all)
			if err != nil {
				a.internal(w, err)
				return
			}
			respond(w, 200, map[string]any{"note": noteView{n, links}, "html": b.String(), "history": history, "history_next_before": nextBefore})
			return
		}
		if len(parts) == 2 && parts[1] == "connections" && r.Method == "POST" {
			var in struct {
				ExpectedRevision int      `json:"expected_revision"`
				IsProjectSummary *bool    `json:"is_project_summary"`
				RelatedTo        []string `json:"related_to"`
				Supersedes       []string `json:"supersedes"`
			}
			if decode(r, &in) != nil {
				fail(w, 400, "Metadata relasi tidak valid.")
				return
			}
			n, err := a.notes.Get(id)
			if err != nil {
				a.noteError(w, err)
				return
			}
			n, err = a.notes.ReviseContext(r.Context(), id, in.ExpectedRevision, memory.Input{
				Project: n.Project, Kind: n.Kind, Title: n.Title, Content: n.Content, Status: n.Status, Source: n.Source,
				IsProjectSummary: in.IsProjectSummary, RelatedTo: in.RelatedTo, Supersedes: in.Supersedes,
			}, "admin")
			if err != nil {
				if errors.Is(err, memory.ErrInvalid) {
					fail(w, 400, "Relasi tidak valid: gunakan ID unik dalam proyek yang sama, maks. 20 per jenis, tanpa diri sendiri atau siklus menggantikan. Ringkasan wajib jenis pengetahuan, maks. 8000 byte.")
					return
				}
				if errors.Is(err, memory.ErrConflict) {
					fail(w, 409, "Revisi berubah, catatan diarsipkan, atau proyek sudah memiliki ringkasan. Muat ulang dan periksa ringkasan proyek.")
					return
				}
				a.noteError(w, err)
				return
			}
			a.audit("admin", "note.connections", id)
			respond(w, 200, n)
			return
		}
		if len(parts) == 2 && r.Method == "POST" && (parts[1] == "publish" || parts[1] == "archive" || parts[1] == "restore") {
			var in struct {
				ExpectedRevision int `json:"expected_revision"`
			}
			if decode(r, &in) != nil {
				fail(w, 400, "Revisi diperlukan.")
				return
			}
			var n memory.Note
			var e error
			if parts[1] == "publish" {
				n, e = a.notes.PublishContext(r.Context(), id, in.ExpectedRevision, "admin")
			} else if parts[1] == "restore" {
				n, e = a.notes.RestoreContext(r.Context(), id, in.ExpectedRevision, "admin")
			} else {
				n, e = a.notes.ArchiveContext(r.Context(), id, in.ExpectedRevision, "admin")
			}
			if e != nil {
				a.noteError(w, e)
				return
			}
			a.audit("admin", "note."+parts[1], id)
			respond(w, 200, n)
			return
		}
		fail(w, 404, "Tidak ditemukan.")
	default:
		fail(w, 404, "Tidak ditemukan.")
	}
}
func matches(n memory.Note, q string) bool {
	return strings.Contains(strings.ToLower(n.Title+"\n"+n.Content+"\n"+n.Source), q)
}
func sortNotes(notes []memory.Note) {
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].UpdatedAt == notes[j].UpdatedAt {
			return notes[i].ID < notes[j].ID
		}
		return notes[i].UpdatedAt > notes[j].UpdatedAt
	})
}
func (a *App) internal(w http.ResponseWriter, e error) {
	if readFailure(w, e) {
		return
	}
	slog.Error("internal operation failed", "error", e)
	fail(w, 500, "Operasi gagal; periksa log server.")
}
func (a *App) noteError(w http.ResponseWriter, e error) {
	if readFailure(w, e) {
		return
	}
	switch {
	case errors.Is(e, memory.ErrNotFound):
		fail(w, 404, "Catatan tidak ditemukan.")
	case errors.Is(e, memory.ErrConflict):
		fail(w, 409, "Revisi sudah berubah. Muat ulang sebelum mencoba.")
	case errors.Is(e, memory.ErrInvalid):
		fail(w, 400, "Isi atau status catatan tidak valid.")
	default:
		a.internal(w, e)
	}
}
func (a *App) audit(actor, action, target string) {
	// A committed Markdown revision must not become an apparent canceled write
	// because its client disconnected. Audit remains bounded best-effort.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if e := a.auth.AuditContext(ctx, actor, action, target); e != nil {
		slog.Error("audit write failed", "action", action, "target", target)
	}
}
