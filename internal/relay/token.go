package relay

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultClientTokenTTL is how long a token minted from the admin page stays
// valid when the operator does not pick a lifetime.
const DefaultClientTokenTTL = 24 * time.Hour

// TokenStore is the client credential: the shared secret a browser or phone
// presents before it may see any machine.
//
// It is a store rather than a plain string because the admin page can rotate it
// while the relay runs. Rotation is the point: the token is the one secret that
// everyone who views the fleet shares, so it is the one most likely to leak, and
// invalidating it must not need a redeploy.
//
// A token may also carry an expiry. That is the second half of the same idea:
// a token that leaks should stop working on its own, without anyone noticing the
// leak first.
type TokenStore struct {
	path string

	mu        sync.RWMutex
	token     string
	expiresAt time.Time // zero means the token does not expire
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

// Get returns the current token.
func (s *TokenStore) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
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
