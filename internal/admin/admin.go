package admin

import (
	"embed"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/CarlOpenLab/term-fleet/internal/relay"
)

//go:embed ui.html
var uiFS embed.FS

// Options configures the admin surface.
type Options struct {
	// Path is the admin.json file. Empty keeps the account in memory only.
	Path string
	// Relay is the live relay whose registry and token this panel drives.
	Relay *relay.Relay
	// PublicURL is the relay URL an operator should paste into a machine, e.g.
	// "wss://relay.example/relay/agent". The relay cannot discover this itself
	// (it may sit behind a proxy on a path), so it is configuration.
	PublicURL string
	// CACertPath is an optional PEM file to hand out for pinning.
	CACertPath string
	// BasePath is where the panel is mounted, default "/admin".
	BasePath string
	// SessionTTL is how long a login lasts, default 12h.
	SessionTTL time.Duration
	Log        *log.Logger
}

// Admin serves the management panel.
type Admin struct {
	opts  Options
	store *Store
	base  string
	login *loginLimiter
}

// New opens the admin store and prepares the panel.
func New(opts Options) (*Admin, error) {
	if opts.Relay == nil {
		return nil, ErrNoRelay
	}
	store, err := OpenStore(opts.Path)
	if err != nil {
		return nil, err
	}
	base := opts.BasePath
	if base == "" {
		base = "/admin"
	}
	base = "/" + strings.Trim(base, "/")
	if opts.SessionTTL <= 0 {
		opts.SessionTTL = 12 * time.Hour
	}
	if opts.Log == nil {
		opts.Log = log.New(io.Discard, "", 0)
	}
	return &Admin{opts: opts, store: store, base: base, login: newLoginLimiter()}, nil
}

// Configured reports whether the admin account exists yet, so main can say
// "open the panel and create one" instead of printing a login hint nobody can use.
func (a *Admin) Configured() bool { return a.store.Configured() }

// Username is the admin name, for logs.
func (a *Admin) Username() string { return a.store.Username() }

// Register mounts the panel on mux. Both "…/admin" and "…/admin/" are handled so
// a plain link without the trailing slash does not turn into a redirect to an
// absolute path — which, behind a path-prefixed proxy, would escape the prefix.
func (a *Admin) Register(mux *http.ServeMux) {
	b := a.base
	mux.HandleFunc("GET "+b, a.handleUI)
	mux.HandleFunc("GET "+b+"/", a.handleUI)
	mux.HandleFunc("GET "+b+"/api/state", a.handleState)
	mux.HandleFunc("POST "+b+"/api/setup", a.handleSetup)
	mux.HandleFunc("POST "+b+"/api/login", a.handleLogin)
	mux.HandleFunc("POST "+b+"/api/logout", a.handleLogout)
	mux.HandleFunc("POST "+b+"/api/password", a.handlePassword)
	mux.HandleFunc("GET "+b+"/api/devices", a.handleDevices)
	mux.HandleFunc("POST "+b+"/api/devices", a.handleEnroll)
	mux.HandleFunc("POST "+b+"/api/devices/revoke", a.handleRevoke)
	mux.HandleFunc("GET "+b+"/api/token", a.handleToken)
	mux.HandleFunc("POST "+b+"/api/token/rotate", a.handleRotate)
	mux.HandleFunc("GET "+b+"/ca.pem", a.handleCA)
}

// ── pages and static ────────────────────────────────────────────────────────

func (a *Admin) handleUI(w http.ResponseWriter, r *http.Request) {
	page, err := uiFS.ReadFile("ui.html")
	if err != nil {
		http.Error(w, "admin: ui unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The panel shows live credentials; never let a proxy or browser cache it.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A panel that mints shell credentials should not be frameable.
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; img-src 'self' data:")
	_, _ = w.Write(page)
}

func (a *Admin) handleCA(w http.ResponseWriter, r *http.Request) {
	if a.opts.CACertPath == "" {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(a.opts.CACertPath)
	if err != nil {
		a.opts.Log.Printf("admin: read ca: %v", err)
		http.Error(w, "admin: ca unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="relay-ca.pem"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// ── session endpoints ───────────────────────────────────────────────────────

type stateReply struct {
	Configured    bool   `json:"configured"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	RelayURL      string `json:"relayUrl,omitempty"`
	CA            bool   `json:"ca"`
	Host          string `json:"host,omitempty"`
}

func (a *Admin) handleState(w http.ResponseWriter, r *http.Request) {
	authed := a.authed(r)
	reply := stateReply{
		Configured:    a.store.Configured(),
		Authenticated: authed,
		RelayURL:      a.relayURL(r),
		CA:            a.opts.CACertPath != "",
		Host:          r.Host,
	}
	if authed {
		reply.Username = a.store.Username()
	}
	writeJSON(w, http.StatusOK, reply)
}

func (a *Admin) handleSetup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	err := a.store.Setup(body.Username, body.Password)
	switch {
	case err == ErrConfigured:
		a.fail(w, http.StatusConflict, "already configured")
		return
	case err != nil:
		a.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	a.opts.Log.Printf("admin: account %q created", a.store.Username())
	if err := a.startSession(w, r); err != nil {
		a.fail(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": a.store.Username()})
}

func (a *Admin) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !a.login.allow(ip) {
		a.fail(w, http.StatusTooManyRequests, "too many attempts; wait a few minutes")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if !a.store.Verify(body.Username, body.Password) {
		a.login.fail(ip)
		a.opts.Log.Printf("admin: failed login for %q from %s", body.Username, ip)
		a.fail(w, http.StatusUnauthorized, "wrong username or password")
		return
	}
	a.login.reset(ip)
	if err := a.startSession(w, r); err != nil {
		a.fail(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": a.store.Username()})
}

func (a *Admin) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		a.store.DropSession(c.Value)
	}
	a.clearCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *Admin) handlePassword(w http.ResponseWriter, r *http.Request) {
	if !a.guard(w, r) {
		return
	}
	var body struct {
		Current string `json:"current"`
		Next    string `json:"next"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if err := a.store.ChangePassword(body.Current, body.Next); err != nil {
		a.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	// ChangePassword dropped every session including ours; issue a fresh one.
	a.clearCookie(w, r)
	if err := a.startSession(w, r); err != nil {
		a.fail(w, http.StatusInternalServerError, "password changed; please log in again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ── fleet endpoints ─────────────────────────────────────────────────────────

type deviceView struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	AddedAt string `json:"addedAt,omitempty"`
	Online  bool   `json:"online"`
}

func (a *Admin) handleDevices(w http.ResponseWriter, r *http.Request) {
	if !a.guard(w, r) {
		return
	}
	online := map[string]bool{}
	for _, m := range a.opts.Relay.Roster() {
		if m.Online {
			online[m.ID] = true
		}
	}
	entries := a.opts.Relay.Devices().Entries()
	out := make([]deviceView, 0, len(entries))
	for _, e := range entries {
		out = append(out, deviceView{ID: e.ID, Name: e.Name, AddedAt: e.AddedAt, Online: online[e.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func (a *Admin) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if !a.guard(w, r) {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		a.fail(w, http.StatusBadRequest, "give the machine a name")
		return
	}
	id, secret, err := a.opts.Relay.Devices().Enroll(name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		a.opts.Log.Printf("admin: enroll %q: %v", name, err)
		a.fail(w, http.StatusInternalServerError, "could not enroll the machine")
		return
	}
	a.opts.Log.Printf("admin: enrolled %s (%s)", id, name)
	a.opts.Relay.RefreshRoster()
	// The secret is in this response and nowhere else; the panel shows it once.
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "secret": secret, "name": name})
}

func (a *Admin) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if !a.guard(w, r) {
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	removed, err := a.opts.Relay.Devices().Revoke(strings.TrimSpace(body.ID))
	if err != nil {
		a.opts.Log.Printf("admin: revoke %q: %v", body.ID, err)
		a.fail(w, http.StatusInternalServerError, "could not revoke the machine")
		return
	}
	a.opts.Log.Printf("admin: revoked %s (removed=%v)", body.ID, removed)
	a.opts.Relay.RefreshRoster()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}

func (a *Admin) handleToken(w http.ResponseWriter, r *http.Request) {
	if !a.guard(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": a.opts.Relay.ClientToken().Get()})
}

func (a *Admin) handleRotate(w http.ResponseWriter, r *http.Request) {
	if !a.guard(w, r) {
		return
	}
	token, err := a.opts.Relay.ClientToken().Rotate()
	if err != nil {
		a.opts.Log.Printf("admin: rotate token: %v", err)
		a.fail(w, http.StatusInternalServerError, "could not rotate the token")
		return
	}
	a.opts.Log.Printf("admin: client token rotated")
	writeJSON(w, http.StatusOK, map[string]any{"token": token})
}

// ── helpers ─────────────────────────────────────────────────────────────────

const cookieName = "tf_admin"

// guard rejects anything that is not an authenticated same-site request. The
// cookie is SameSite=Strict, so a cross-site POST cannot carry it at all; the
// Origin check is the second lock on the same door, for browsers or proxies that
// pass the cookie anyway.
func (a *Admin) guard(w http.ResponseWriter, r *http.Request) bool {
	if !a.authed(r) {
		a.fail(w, http.StatusUnauthorized, "not signed in")
		return false
	}
	if !sameOrigin(r) {
		a.fail(w, http.StatusForbidden, "cross-site request rejected")
		return false
	}
	return true
}

func (a *Admin) authed(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	return a.store.Session(c.Value)
}

func (a *Admin) startSession(w http.ResponseWriter, r *http.Request) error {
	tok, err := a.store.NewSession(a.opts.SessionTTL)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tok,
		Path:     a.base,
		HttpOnly: true,
		Secure:   a.secure(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(a.opts.SessionTTL.Seconds()),
	})
	return nil
}

func (a *Admin) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     a.base,
		HttpOnly: true,
		Secure:   a.secure(r),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// secure marks the cookie Secure whenever the browser reached us over TLS,
// including when TLS was terminated by the proxy in front.
func (a *Admin) secure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (a *Admin) relayURL(r *http.Request) string {
	if a.opts.PublicURL != "" {
		return a.opts.PublicURL
	}
	// A guess that is usually right behind a plain proxy, and is always shown as
	// a starting point rather than applied silently.
	scheme := "wss"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "ws"
	}
	return scheme + "://" + r.Host + "/relay/agent"
}

func (a *Admin) fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	if err := dec.Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed request"})
		return false
	}
	return true
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// No Origin: a same-origin fetch from some browsers, or a curl. The
		// session cookie is SameSite=Strict, so this cannot be cross-site.
		return true
	}
	return strings.EqualFold(origin, "http://"+r.Host) || strings.EqualFold(origin, "https://"+r.Host)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// loginLimiter throttles password guessing per client IP.
type loginLimiter struct {
	mu       sync.Mutex
	fails    map[string][]time.Time
	max      int
	window   time.Duration
	cooldown time.Duration
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{fails: map[string][]time.Time{}, max: 8, window: 15 * time.Minute, cooldown: 15 * time.Minute}
}

func (l *loginLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	kept := l.fails[ip][:0]
	for _, t := range l.fails[ip] {
		if now.Sub(t) < l.window {
			kept = append(kept, t)
		}
	}
	l.fails[ip] = kept
	if len(kept) >= l.max {
		// Locked until the oldest failure falls out of the window.
		return now.Sub(kept[0]) > l.cooldown
	}
	return true
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[ip] = append(l.fails[ip], time.Now())
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}
