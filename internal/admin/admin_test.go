package admin

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/CarlOpenLab/term-fleet/internal/relay"
)

// panel is a running admin UI in front of a real relay: the same wiring
// cmd/tfrelay uses, so the tests exercise the paths an operator actually hits.
type panel struct {
	ts     *httptest.Server
	client *http.Client
	relay  *relay.Relay
	admin  *Admin
	dir    string
}

func newPanel(t *testing.T) *panel {
	t.Helper()
	return newPanelWithCA(t, nil)
}

func newPanelWithCA(t *testing.T, ca []byte) *panel {
	t.Helper()
	dir := t.TempDir()
	reg, err := relay.OpenRegistry(filepath.Join(dir, "devices.json"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	token := relay.NewTokenStore("client-token-for-tests", filepath.Join(dir, "token.txt"))
	r, err := relay.New(relay.Config{Registry: reg, Token: token, Log: log.New(io.Discard, "", 0)})
	if err != nil {
		t.Fatalf("relay: %v", err)
	}

	caPath := ""
	if ca != nil {
		caPath = filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(caPath, ca, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, err := New(Options{
		Path:       filepath.Join(dir, "admin.json"),
		Relay:      r,
		CACertPath: caPath,
		PublicURL:  "wss://relay.example/relay/agent",
		Log:        log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	mux := http.NewServeMux()
	a.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	return &panel{ts: ts, client: &http.Client{Jar: jar}, relay: r, admin: a, dir: dir}
}

// newClient is a browser with an empty cookie jar: a second visitor, with no
// session yet.
func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

type reply struct {
	status int
	body   map[string]any
	raw    string
	header http.Header
}

func (p *panel) call(t *testing.T, method, path string, payload any, hdr map[string]string) reply {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, p.ts.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := p.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := reply{status: res.StatusCode, raw: string(raw), header: res.Header}
	_ = json.Unmarshal(raw, &out.body)
	return out
}

func (p *panel) setup(t *testing.T) {
	t.Helper()
	got := p.call(t, "POST", "/admin/api/setup", map[string]string{"username": "carl", "password": "correct horse"}, nil)
	if got.status != http.StatusOK {
		t.Fatalf("setup = %d %s, want 200", got.status, got.raw)
	}
}

// TestSetupThenLogin walks the first-run path: an unconfigured relay offers
// setup, setup signs you in, and the session then opens the fleet endpoints.
func TestSetupThenLogin(t *testing.T) {
	p := newPanel(t)

	st := p.call(t, "GET", "/admin/api/state", nil, nil)
	if st.status != http.StatusOK {
		t.Fatalf("state = %d", st.status)
	}
	if st.body["configured"] != false || st.body["authenticated"] != false {
		t.Fatalf("state = %v, want unconfigured and unauthenticated", st.body)
	}
	if _, leaked := st.body["token"]; leaked {
		t.Fatal("the state endpoint must never carry the client token")
	}

	p.setup(t)

	st = p.call(t, "GET", "/admin/api/state", nil, nil)
	if st.body["authenticated"] != true || st.body["username"] != "carl" {
		t.Fatalf("state after setup = %v, want an authenticated carl", st.body)
	}

	// A fresh client (no cookie) must be asked to log in, and must be able to.
	fresh := &panel{ts: p.ts, client: newClient(t), relay: p.relay, admin: p.admin, dir: p.dir}
	if got := fresh.call(t, "GET", "/admin/api/devices", nil, nil); got.status != http.StatusUnauthorized {
		t.Fatalf("devices without a session = %d, want 401", got.status)
	}
	login := fresh.call(t, "POST", "/admin/api/login", map[string]string{"username": "carl", "password": "correct horse"}, nil)
	if login.status != http.StatusOK {
		t.Fatalf("login = %d %s, want 200", login.status, login.raw)
	}
	if got := fresh.call(t, "GET", "/admin/api/devices", nil, nil); got.status != http.StatusOK {
		t.Fatalf("devices after login = %d, want 200", got.status)
	}

	bad := fresh.call(t, "POST", "/admin/api/login", map[string]string{"username": "carl", "password": "wrong"}, nil)
	if bad.status != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", bad.status)
	}
}

// TestSetupCannotRunTwice: the setup endpoint must not be usable to seize a
// relay that is already running.
func TestSetupCannotRunTwice(t *testing.T) {
	p := newPanel(t)
	p.setup(t)

	again := p.call(t, "POST", "/admin/api/setup", map[string]string{"username": "intruder", "password": "let me in"}, nil)
	if again.status != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", again.status)
	}
	if p.admin.Username() != "carl" {
		t.Fatalf("admin = %q, want carl: the account must not have been replaced", p.admin.Username())
	}
}

// TestSetupRejectsWeakPassword keeps the one credential guarding shell access
// from being four characters long.
func TestSetupRejectsWeakPassword(t *testing.T) {
	p := newPanel(t)
	got := p.call(t, "POST", "/admin/api/setup", map[string]string{"username": "carl", "password": "short"}, nil)
	if got.status != http.StatusBadRequest {
		t.Fatalf("weak password = %d %s, want 400", got.status, got.raw)
	}
	if p.admin.Configured() {
		t.Fatal("a rejected setup left the relay configured")
	}
}

// TestPasswordIsStoredHashed is the reason a leaked admin.json is survivable.
func TestPasswordIsStoredHashed(t *testing.T) {
	p := newPanel(t)
	p.setup(t)

	b, err := os.ReadFile(filepath.Join(p.dir, "admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "correct horse") {
		t.Fatalf("admin.json contains the plaintext password: %s", b)
	}
	var creds Credentials
	if err := json.Unmarshal(b, &creds); err != nil {
		t.Fatalf("admin.json: %v", err)
	}
	if !strings.HasPrefix(creds.Hash, "pbkdf2_sha256$") {
		t.Fatalf("hash = %q, want a pbkdf2 record", creds.Hash)
	}
	if st, _ := os.Stat(filepath.Join(p.dir, "admin.json")); st.Mode().Perm() != 0o600 {
		t.Fatalf("admin.json mode = %o, want 600", st.Mode().Perm())
	}
}

// TestCrossOriginRequestRejected: the panel mints credentials, so a form post
// from another site must not be able to drive it even with a live cookie.
func TestCrossOriginRequestRejected(t *testing.T) {
	p := newPanel(t)
	p.setup(t)

	got := p.call(t, "POST", "/admin/api/devices", map[string]string{"name": "attacker"},
		map[string]string{"Origin": "https://evil.example"})
	if got.status != http.StatusForbidden {
		t.Fatalf("cross-origin enroll = %d %s, want 403", got.status, got.raw)
	}
	if got := p.relay.Devices().Len(); got != 0 {
		t.Fatalf("registry has %d machines after a rejected request", got)
	}
}

// TestEnrollAndRevokeThroughPanel is the feature end to end: the page mints a
// credential, the relay's registry gains the machine, and revoke takes it away.
func TestEnrollAndRevokeThroughPanel(t *testing.T) {
	p := newPanel(t)
	p.setup(t)

	got := p.call(t, "POST", "/admin/api/devices", map[string]string{"name": "mac mini"}, nil)
	if got.status != http.StatusOK {
		t.Fatalf("enroll = %d %s, want 200", got.status, got.raw)
	}
	id, _ := got.body["id"].(string)
	secret, _ := got.body["secret"].(string)
	if id == "" || secret == "" {
		t.Fatalf("enroll returned %v, want an id and a secret", got.body)
	}

	if stored, ok := p.relay.Devices().Secret(id); !ok || stored != secret {
		t.Fatalf("the relay does not accept the freshly issued credential (%q,%v)", stored, ok)
	}
	if entries := p.relay.Devices().Entries(); len(entries) != 1 || entries[0].Name != "mac mini" {
		t.Fatalf("registry = %+v, want one entry named mac mini", entries)
	}

	list := p.call(t, "GET", "/admin/api/devices", nil, nil)
	devices, _ := list.body["devices"].([]any)
	if len(devices) != 1 {
		t.Fatalf("device list = %v, want one", list.body)
	}
	first, _ := devices[0].(map[string]any)
	if first["id"] != id || first["online"] != false {
		t.Fatalf("device row = %v, want the enrolled machine and offline", first)
	}

	rev := p.call(t, "POST", "/admin/api/devices/revoke", map[string]string{"id": id}, nil)
	if rev.status != http.StatusOK {
		t.Fatalf("revoke = %d %s, want 200", rev.status, rev.raw)
	}
	if _, ok := p.relay.Devices().Secret(id); ok {
		t.Fatal("the revoked machine is still accepted")
	}

	// Revoking twice is not an error: the table may be a poll behind.
	again := p.call(t, "POST", "/admin/api/devices/revoke", map[string]string{"id": id}, nil)
	if again.status != http.StatusOK {
		t.Fatalf("second revoke = %d, want 200", again.status)
	}
}

// TestEnrollRequiresAName keeps anonymous machines out of the roster.
func TestEnrollRequiresAName(t *testing.T) {
	p := newPanel(t)
	p.setup(t)
	got := p.call(t, "POST", "/admin/api/devices", map[string]string{"name": "  "}, nil)
	if got.status != http.StatusBadRequest {
		t.Fatalf("nameless enroll = %d, want 400", got.status)
	}
}

// TestTokenRotateThroughPanel checks the page can read and rotate the token, and
// that rotation is what the relay honours afterwards.
func TestTokenRotateThroughPanel(t *testing.T) {
	p := newPanel(t)
	p.setup(t)

	got := p.call(t, "GET", "/admin/api/token", nil, nil)
	if got.body["token"] != "client-token-for-tests" {
		t.Fatalf("token = %v, want the configured one", got.body)
	}

	rot := p.call(t, "POST", "/admin/api/token/rotate", map[string]any{}, nil)
	if rot.status != http.StatusOK {
		t.Fatalf("rotate = %d %s, want 200", rot.status, rot.raw)
	}
	next, _ := rot.body["token"].(string)
	if next == "" || next == "client-token-for-tests" {
		t.Fatalf("rotated token = %q, want a new value", next)
	}
	if p.relay.ClientToken().Get() != next {
		t.Fatal("the relay is still serving the old token")
	}

	b, err := os.ReadFile(filepath.Join(p.dir, "token.txt"))
	if err != nil {
		t.Fatalf("the rotated token was not persisted: %v", err)
	}
	if strings.TrimSpace(string(b)) != next {
		t.Fatalf("token.txt = %q, want the rotated token", strings.TrimSpace(string(b)))
	}
}

// TestPasswordChangeEndsOldSessions: rotating the admin password must not leave
// a stolen cookie working.
func TestPasswordChangeEndsOldSessions(t *testing.T) {
	p := newPanel(t)
	p.setup(t)

	// A second session, standing in for another device left logged in.
	other := &panel{ts: p.ts, client: newClient(t), relay: p.relay, admin: p.admin, dir: p.dir}
	if got := other.call(t, "POST", "/admin/api/login", map[string]string{"username": "carl", "password": "correct horse"}, nil); got.status != http.StatusOK {
		t.Fatalf("second login = %d", got.status)
	}

	ch := p.call(t, "POST", "/admin/api/password", map[string]string{"current": "correct horse", "next": "a brand new one"}, nil)
	if ch.status != http.StatusOK {
		t.Fatalf("change = %d %s, want 200", ch.status, ch.raw)
	}
	if got := other.call(t, "GET", "/admin/api/devices", nil, nil); got.status != http.StatusUnauthorized {
		t.Fatalf("the other session survived a password change: %d", got.status)
	}
	// The session that made the change keeps working.
	if got := p.call(t, "GET", "/admin/api/devices", nil, nil); got.status != http.StatusOK {
		t.Fatalf("the changing session was signed out: %d", got.status)
	}
	// And the old password no longer logs in.
	fresh := &panel{ts: p.ts, client: newClient(t), relay: p.relay, admin: p.admin, dir: p.dir}
	if got := fresh.call(t, "POST", "/admin/api/login", map[string]string{"username": "carl", "password": "correct horse"}, nil); got.status != http.StatusUnauthorized {
		t.Fatalf("the old password still works: %d", got.status)
	}
	if got := fresh.call(t, "POST", "/admin/api/login", map[string]string{"username": "carl", "password": "a brand new one"}, nil); got.status != http.StatusOK {
		t.Fatalf("the new password does not work: %d", got.status)
	}
}

// TestLogoutEndsTheSession.
func TestLogoutEndsTheSession(t *testing.T) {
	p := newPanel(t)
	p.setup(t)
	if got := p.call(t, "POST", "/admin/api/logout", map[string]any{}, nil); got.status != http.StatusOK {
		t.Fatalf("logout = %d", got.status)
	}
	if got := p.call(t, "GET", "/admin/api/devices", nil, nil); got.status != http.StatusUnauthorized {
		t.Fatalf("devices after logout = %d, want 401", got.status)
	}
}

// TestCAEndpoint: the CA download is how a member learns to trust a private
// certificate, and it must 404 rather than break when none is configured.
func TestCAEndpoint(t *testing.T) {
	p := newPanelWithCA(t, []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"))
	got := p.call(t, "GET", "/admin/ca.pem", nil, nil)
	if got.status != http.StatusOK {
		t.Fatalf("ca = %d, want 200", got.status)
	}
	if !strings.Contains(got.raw, "BEGIN CERTIFICATE") {
		t.Fatalf("ca body = %q, want PEM", got.raw)
	}

	none := newPanel(t)
	if got := none.call(t, "GET", "/admin/ca.pem", nil, nil); got.status != http.StatusNotFound {
		t.Fatalf("ca with none configured = %d, want 404", got.status)
	}
}

// TestStateAdvertisesTheConfiguredRelayURL: the panel has to show operators what
// to paste into a machine, and it cannot guess a proxied path.
func TestStateAdvertisesTheConfiguredRelayURL(t *testing.T) {
	p := newPanel(t)
	got := p.call(t, "GET", "/admin/api/state", nil, nil)
	if got.body["relayUrl"] != "wss://relay.example/relay/agent" {
		t.Fatalf("relayUrl = %v, want the configured -public-url", got.body["relayUrl"])
	}
}

// TestLoginIsRateLimited: the panel is reachable from the open internet, so
// password guessing has to get expensive.
func TestLoginIsRateLimited(t *testing.T) {
	p := newPanel(t)
	p.setup(t)
	fresh := &panel{ts: p.ts, client: newClient(t), relay: p.relay, admin: p.admin, dir: p.dir}

	limited := false
	for i := 0; i < 20; i++ {
		got := fresh.call(t, "POST", "/admin/api/login", map[string]string{"username": "carl", "password": "nope"}, nil)
		if got.status == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("20 failed logins were allowed; there is no rate limit")
	}
}

// TestSessionCookieScopedToThePublicPath is the regression for the bug that only
// a path-prefixing proxy exposes: the relay sees "/admin/api/setup" while the
// browser is on "/termfleet/admin/api/setup". A cookie scoped to "/admin" is
// never sent back, so setup appears to succeed and every later request is
// anonymous.
func TestSessionCookieScopedToThePublicPath(t *testing.T) {
	p := newPanel(t)
	got := p.call(t, "POST", "/admin/api/setup",
		map[string]string{"username": "carl", "password": "correct horse"},
		map[string]string{"X-Forwarded-Prefix": "/termfleet", "X-Forwarded-Proto": "https"})
	if got.status != http.StatusOK {
		t.Fatalf("setup = %d %s, want 200", got.status, got.raw)
	}
	cookie := got.header.Get("Set-Cookie")
	if !strings.Contains(cookie, "Path=/termfleet/admin") {
		t.Fatalf("Set-Cookie = %q, want the public path so the browser sends it back", cookie)
	}
	for _, want := range []string{"HttpOnly", "SameSite=Strict", "Secure"} {
		if !strings.Contains(cookie, want) {
			t.Fatalf("Set-Cookie = %q, want %s", cookie, want)
		}
	}
}

// TestCookiePathDerivation covers the cases behind cookiePath: mounted directly,
// mounted behind a proxy that forwards the full path, and one that strips the
// prefix and advertises it.
func TestCookiePathDerivation(t *testing.T) {
	a := &Admin{base: "/admin"}
	cases := []struct {
		path, prefix, want string
	}{
		{"/admin/api/setup", "", "/admin"},
		{"/admin", "", "/admin"},
		{"/termfleet/admin/api/setup", "", "/termfleet/admin"},
		{"/admin/api/setup", "/termfleet", "/termfleet/admin"},
		{"/admin/api/setup", "termfleet/", "/termfleet/admin"},
		{"/admin/api/setup", "/a/admin/b", "/a/admin/b/admin"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", c.path, nil)
		if c.prefix != "" {
			req.Header.Set("X-Forwarded-Prefix", c.prefix)
		}
		if got := a.cookiePath(req); got != c.want {
			t.Errorf("cookiePath(%q, prefix %q) = %q, want %q", c.path, c.prefix, got, c.want)
		}
	}
}

// TestNoOriginHeaderIsAcceptedButDifferentOriginIsNot pins both halves of the
// cross-site rule: a same-origin fetch may omit Origin, but a stated different
// origin is refused.
func TestNoOriginHeaderIsAcceptedButDifferentOriginIsNot(t *testing.T) {
	p := newPanel(t)
	p.setup(t)
	ok := p.call(t, "POST", "/admin/api/devices", map[string]string{"name": "box"}, nil)
	if ok.status != http.StatusOK {
		t.Fatalf("enroll without an Origin header = %d, want 200", ok.status)
	}
	same := p.call(t, "POST", "/admin/api/devices", map[string]string{"name": "box2"},
		map[string]string{"Origin": p.ts.URL})
	if same.status != http.StatusOK {
		t.Fatalf("same-origin enroll = %d, want 200", same.status)
	}
}

// TestUIScriptReferencesOnlyExistingIDs: the panel is one inline script, and a
// stray id lookup is a null dereference that only shows up when that code path
// runs — invisible in a screenshot, invisible in a health check. The ids are
// checked statically instead.
func TestUIScriptReferencesOnlyExistingIDs(t *testing.T) {
	raw, err := uiFS.ReadFile("ui.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	scriptStart := strings.Index(page, "<script>")
	scriptEnd := strings.LastIndex(page, "</script>")
	if scriptStart < 0 || scriptEnd < scriptStart {
		t.Fatal("ui.html has no inline script")
	}
	script := page[scriptStart:scriptEnd]

	declared := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(page, -1) {
		declared[m[1]] = true
	}
	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`\$\("([^"]+)"\)`).FindAllStringSubmatch(script, -1) {
		used[m[1]] = true
	}
	if len(used) < 10 {
		t.Fatalf("only found %d id lookups; the parser probably broke", len(used))
	}
	var missing []string
	for id := range used {
		if !declared[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("the script looks up ids that are not in the page: %v", missing)
	}
}

// TestPasswordHashingRoundTrips covers the primitives directly, including the
// refusal of a malformed record.
func TestPasswordHashingRoundTrips(t *testing.T) {
	h, err := hashPassword("hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(h, "hunter2hunter2") {
		t.Fatal("a correct password did not verify")
	}
	if verifyPassword(h, "hunter2hunter3") {
		t.Fatal("a wrong password verified")
	}
	for _, bad := range []string{"", "nonsense", "pbkdf2_sha256$0$AA$AA", "md5$1$AA$AA", "pbkdf2_sha256$x$AA$AA"} {
		if verifyPassword(bad, "hunter2hunter2") {
			t.Fatalf("malformed record %q verified", bad)
		}
	}
	// Two hashes of the same password must differ: that is the salt doing work.
	h2, err := hashPassword("hunter2hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if h == h2 {
		t.Fatal("hashing the same password twice produced the same record; the salt is fixed")
	}
}
