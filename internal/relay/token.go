package relay

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultClientTokenTTL is how long a token minted from the admin page stays
// valid when the operator does not pick a lifetime.
const DefaultClientTokenTTL = 24 * time.Hour

// DefaultControlTokenTTL is how long a credential minted for an enrolled machine
// stays valid when the relay is not told otherwise. Short on purpose: it is
// handed to a machine so that a window can drive the rest of the fleet, and a
// machine that keeps asking is a machine that still wants to.
const DefaultControlTokenTTL = 15 * time.Minute

// TokenStore is the client credential: the secret a client presents before it
// may see any machine.
//
// It is a store rather than a plain string because the admin page can rotate the
// shared token while the relay runs. Rotation is the point: the shared token is
// the one secret everyone who views the fleet holds, so it is the one most
// likely to leak, and invalidating it must not need a redeploy.
//
// A token may carry an expiry, and that is the second half of the same idea: a
// token that leaks should stop working on its own, without anyone noticing the
// leak first. The shared token's expiry is the admin page's; the minted ones
// below are handed to enrolled machines and are always short-lived.
type TokenStore struct {
	path string

	mu        sync.RWMutex
	token     string
	expiresAt time.Time // zero means the token does not expire
	// minted are credentials issued to enrolled machines, keyed by the token
	// itself: a machine asks the relay for one (ControlToken) instead of being
	// handed the shared secret, so revoking a machine, or simply waiting, takes
	// its reach away. They are kept in memory only — a credential that outlives
	// the process that asked for it is longer lived than anything that needs it —
	// and each remembers which machine asked, so an operator can see who was
	// handed one.
	minted map[string]mintedToken
}

// mintedToken is one credential handed to an enrolled machine.
type mintedToken struct {
	device    string
	expiresAt time.Time
}

// MintedToken is one live machine credential, as the admin page and the tests
// see it.
type MintedToken struct {
	Device    string    `json:"device"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// NewTokenStore wraps an initial token. When path is set the token is persisted
// there on rotation, so a restart honours the rotation instead of resurrecting
// the old value.
//
// A token supplied this way is taken as deliberately configured and does not
// expire; lifetimes belong to tokens the admin page mints.
func NewTokenStore(token, path string) *TokenStore {
	return &TokenStore{path: path, token: token}
}

// tokenFile is the persisted shape. Relays older than expiry wrote the bare
// token on one line, and OpenTokenStore still accepts that form so upgrading
// does not lock every viewer out.
type tokenFile struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// OpenTokenStore loads a persisted token, falling back to initial when no file
// exists yet.
func OpenTokenStore(path, initial string) (*TokenStore, error) {
	s := &TokenStore{path: path, token: initial}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	body := strings.TrimSpace(string(b))
	if body == "" {
		return s, nil
	}
	var f tokenFile
	if err := json.Unmarshal(b, &f); err == nil && strings.TrimSpace(f.Token) != "" {
		s.token = strings.TrimSpace(f.Token)
		if f.ExpiresAt != "" {
			exp, err := time.Parse(time.RFC3339, f.ExpiresAt)
			if err != nil {
				// A credential we cannot date is treated as spent: the admin
				// page can always mint a fresh one, and failing open would let
				// a corrupted file hand out eternal access.
				exp = time.Unix(1, 0)
			}
			s.expiresAt = exp
		}
		return s, nil
	}
	// Legacy plain-text file: a token with no expiry.
	if !strings.HasPrefix(body, "{") {
		s.token = body
	}
	return s, nil
}

// Get returns the current shared token.
func (s *TokenStore) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

// Accept reports whether a presented credential is one this relay honours: the
// shared token while it has not expired, or a live credential minted for an
// enrolled machine. Comparisons are constant time, so a caller cannot learn a
// token one byte at a time.
func (s *TokenStore) Accept(got string) bool {
	if got == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	if s.token != "" && (s.expiresAt.IsZero() || now.Before(s.expiresAt)) {
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1 {
			return true
		}
	}
	for token, m := range s.minted {
		if now.Before(m.expiresAt) && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1 {
			return true
		}
	}
	return false
}

// Mint issues a credential to the machine named device, good for ttl (a ttl
// <= 0 gets DefaultControlTokenTTL). Expired credentials are dropped on the way
// through, so the set stays as small as the machines actually asking for one.
func (s *TokenStore) Mint(device string, ttl time.Duration) (string, time.Time, error) {
	if ttl <= 0 {
		ttl = DefaultControlTokenTTL
	}
	token, err := newSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := time.Now().Add(ttl)

	s.mu.Lock()
	now := time.Now()
	for t, m := range s.minted {
		if !now.Before(m.expiresAt) {
			delete(s.minted, t)
		}
	}
	if s.minted == nil {
		s.minted = map[string]mintedToken{}
	}
	s.minted[token] = mintedToken{device: device, expiresAt: expiresAt}
	s.mu.Unlock()
	return token, expiresAt, nil
}

// Minted lists the live machine credentials, oldest expiry first.
func (s *TokenStore) Minted() []MintedToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	out := make([]MintedToken, 0, len(s.minted))
	for _, m := range s.minted {
		if now.Before(m.expiresAt) {
			out = append(out, MintedToken{Device: m.device, ExpiresAt: m.expiresAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt.Before(out[j].ExpiresAt) })
	return out
}

// ForgetDevice drops every live credential minted to device. It is what makes
// revoking a machine mean something immediately rather than at the next expiry.
func (s *TokenStore) ForgetDevice(device string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for t, m := range s.minted {
		if m.device == device {
			delete(s.minted, t)
			n++
		}
	}
	return n
}

// ExpiresAt reports when the token stops being accepted. The zero time means it
// does not expire.
func (s *TokenStore) ExpiresAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.expiresAt
}

// Expired reports whether the token is past its expiry.
func (s *TokenStore) Expired() bool {
	exp := s.ExpiresAt()
	return !exp.IsZero() && !time.Now().Before(exp)
}

// TTL is the time left before the token expires, or zero when it never does.
func (s *TokenStore) TTL() time.Duration {
	exp := s.ExpiresAt()
	if exp.IsZero() {
		return 0
	}
	return time.Until(exp)
}

// Set replaces the token, persisting it when a path is configured. A ttl <= 0
// installs a token that does not expire.
func (s *TokenStore) Set(token string, ttl time.Duration) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("relay: refusing to set an empty client token")
	}
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}
	if s.path != "" {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
			return err
		}
		f := tokenFile{Token: token}
		if !expiresAt.IsZero() {
			f.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
		}
		b, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			return err
		}
		tmp := s.path + ".tmp"
		if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, s.path); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.token = token
	s.expiresAt = expiresAt
	s.mu.Unlock()
	return nil
}

// Rotate mints a fresh token good for ttl (<= 0 for no expiry) and installs it.
func (s *TokenStore) Rotate(ttl time.Duration) (string, error) {
	t, err := newSecret()
	if err != nil {
		return "", err
	}
	if err := s.Set(t, ttl); err != nil {
		return "", err
	}
	return t, nil
}
