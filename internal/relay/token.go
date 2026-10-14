package relay

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TokenStore is the client credential: the shared secret a browser or phone
// presents before it may see any machine.
//
// It is a store rather than a plain string because the admin page can rotate it
// while the relay runs. Rotation is the point: the token is the one secret that
// everyone who views the fleet shares, so it is the one most likely to leak, and
// invalidating it must not need a redeploy.
type TokenStore struct {
	path string

	mu    sync.RWMutex
	token string
}

// NewTokenStore wraps an initial token. When path is set the token is persisted
// there on rotation, so a restart honours the rotation instead of resurrecting
// the old value.
func NewTokenStore(token, path string) *TokenStore {
	return &TokenStore{path: path, token: token}
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
	if t := strings.TrimSpace(string(b)); t != "" {
		s.token = t
	}
	return s, nil
}

// Get returns the current token.
func (s *TokenStore) Get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

// Set replaces the token, persisting it when a path is configured.
func (s *TokenStore) Set(token string) error {
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("relay: refusing to set an empty client token")
	}
	if s.path != "" {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
			return err
		}
		tmp := s.path + ".tmp"
		if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
			return err
		}
		if err := os.Rename(tmp, s.path); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
	return nil
}

// Rotate mints a fresh token and installs it.
func (s *TokenStore) Rotate() (string, error) {
	t, err := newSecret()
	if err != nil {
		return "", err
	}
	if err := s.Set(t); err != nil {
		return "", err
	}
	return t, nil
}
