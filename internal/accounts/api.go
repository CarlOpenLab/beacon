package accounts

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/CarlOpenLab/beacon/internal/relay"
)

// Service mounts the account API a browser console talks to: an open signup, a
// login that hands back a tenant-bound token, and a self-serve enrollment that
// mints machine credentials into the caller's own tenant.
//
// Everything is Bearer-token or open; no cookies. That keeps the API out of
// CSRF's reach — a cross-site page cannot read another origin's localStorage to
// attach the token — and lets the same handlers serve a plain HTTP client.
type Service struct {
	store *Store
	relay *relay.Relay
	log   *log.Logger

	loginLim *limiter
	regLim   *limiter
}

// New builds the API surface. relay may be nil, which leaves self-serve
// enrollment unmounted (useful in tests that only exercise accounts).
func New(store *Store, rl *relay.Relay, lg *log.Logger) *Service {
	if lg == nil {
		lg = log.New(io.Discard, "", 0)
	}
	return &Service{
		store:    store,
		relay:    rl,
		log:      lg,
		loginLim: newLimiter(8, 15*time.Minute),
		regLim:   newLimiter(5, time.Hour),
	}
}

// Register mounts the endpoints on mux.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/register", s.handleRegister)
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("GET /api/me", s.handleMe)
	if s.relay != nil {
		mux.HandleFunc("POST /api/enroll", s.handleEnroll)
	}
}

// handleRegister creates a pending account. It is open to the internet by
// design, so it is the one endpoint that needs a rate limit on success too: an
// IP that signs up a warehouse of users is filling users.json one POST at a
// time.
func (s *Service) handleRegister(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.regLim.allow(ip) {
		fail(w, http.StatusTooManyRequests, "注册太频繁，请稍后再试")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	u, err := s.store.Register(body.Username, body.Password)
	if err != nil {
		s.regLim.fail(ip)
		switch err {
		case ErrTaken:
			fail(w, http.StatusConflict, "该用户名已被注册")
		case ErrBadUsername:
			fail(w, http.StatusBadRequest, "用户名需为 3-32 位字母、数字或 . - _")
		case ErrBadPassword:
			fail(w, http.StatusBadRequest, "密码至少 8 位")
		case ErrFull:
			fail(w, http.StatusForbidden, "本服务暂不接受更多注册")
		default:
			fail(w, http.StatusInternalServerError, "注册失败")
		}
		return
	}
	s.regLim.reset(ip)
	s.log.Printf("accounts: signup %q (%s) from %s — waiting for approval", u.Username, u.ID, ip)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "pending": true, "username": u.Username})
}

// handleLogin verifies credentials and returns a bearer token bound to the
// caller's tenant. A pending or rejected account is refused with its own
// message: "waiting" and "refused" ask for different actions than "wrong
// password" does.
func (s *Service) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.loginLim.allow(ip) {
		fail(w, http.StatusTooManyRequests, "尝试太频繁，请几分钟后再试")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	u, err := s.store.Authenticate(body.Username, body.Password)
	if err != nil {
		s.loginLim.fail(ip)
		s.log.Printf("accounts: refused login for %q from %s: %v", body.Username, ip, err)
		switch err {
		case ErrPending:
			fail(w, http.StatusForbidden, "账号待管理员审批，审批通过后即可登录")
		case ErrRejected:
			fail(w, http.StatusForbidden, "账号未获批准")
		default:
			fail(w, http.StatusUnauthorized, "用户名或密码错误")
		}
		return
	}
	s.loginLim.reset(ip)
	token, expiresAt, err := s.store.LoginToken(u)
	if err != nil {
		fail(w, http.StatusInternalServerError, "登录失败")
		return
	}
	s.log.Printf("accounts: login %q (%s) from %s", u.Username, u.ID, ip)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"token":     token,
		"expiresAt": expiresAt.UTC().Format(time.RFC3339),
		"username":  u.Username,
		"tenant":    u.ID,
	})
}

// handleMe answers "who am I" for a stored token, so a console can greet the
// user without asking for credentials again.
func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := s.store.VerifyLoginToken(bearer(r))
	if !ok {
		fail(w, http.StatusUnauthorized, "登录已失效")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": user, "tenant": tenant})
}

// handleEnroll is self-serve onboarding for an approved user: the same mint a
// credential flow the admin panel offers, with the tenant taken from the
// token rather than chosen by an operator. The secret is shown once, pasted
// into that machine's desktop client, and never restorable.
func (s *Service) handleEnroll(w http.ResponseWriter, r *http.Request) {
	tenant, user, ok := s.store.VerifyLoginToken(bearer(r))
	if !ok {
		fail(w, http.StatusUnauthorized, "登录已失效")
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
		fail(w, http.StatusBadRequest, "请给机器起个名字")
		return
	}
	id, secret, err := s.relay.Devices().Enroll(name, tenant, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		s.log.Printf("accounts: enroll for %q: %v", user, err)
		fail(w, http.StatusInternalServerError, "无法登记这台机器")
		return
	}
	s.log.Printf("accounts: enrolled %s (%s) into tenant %s for %q", id, name, tenant, user)
	s.relay.RefreshRoster()
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       id,
		"secret":   secret,
		"name":     name,
		"tenant":   tenant,
		"agentUrl": agentURL(r),
	})
}

// agentURL guesses the member dial-in address from the request, the same way
// the admin panel guesses the relay URL.
func agentURL(r *http.Request) string {
	scheme := "wss"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "ws"
	}
	return scheme + "://" + r.Host + "/relay/agent"
}

// bearer reads an Authorization: Bearer credential.
func bearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// ── small shared plumbing ───────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	if err := dec.Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "请求格式错误")
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// limiter throttles attempts per client IP — password guessing on login, and
// json-filling on register. The same shape the admin panel's login limiter
// uses: a rolling window of failures, then a cooldown.
type limiter struct {
	mu       sync.Mutex
	fails    map[string][]time.Time
	max      int
	window   time.Duration
	cooldown time.Duration
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{fails: map[string][]time.Time{}, max: max, window: window, cooldown: window}
}

func (l *limiter) allow(ip string) bool {
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
		return now.Sub(kept[0]) > l.cooldown
	}
	return true
}

func (l *limiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[ip] = append(l.fails[ip], time.Now())
}

func (l *limiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}
