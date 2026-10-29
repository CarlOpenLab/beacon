// Package admin is the relay's web management surface: a first-run setup wizard,
// a login, and from then on a page that mints machine credentials, rotates the
// client token, and hands out the CA certificate.
//
// It exists because the alternative — ssh into the relay box and run
// `tfrelay enroll` for every machine — puts the one operation an operator does
// most often behind the one channel they have least access to.
package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CarlOpenLab/beacon/internal/passhash"
)

// ErrConfigured means setup has already run; there is exactly one admin.
var ErrConfigured = errors.New("admin: already configured")

// ErrNoRelay is a programming error rather than an operator one: the panel has
// nothing to manage without a relay to manage.
var ErrNoRelay = errors.New("admin: a relay is required")

// Credentials is what gets persisted. Only the hash is stored: an operator who
// loses the password resets it by deleting this file and re-running setup.
type Credentials struct {
	Username  string `json:"username"`
	Hash      string `json:"hash"`
	CreatedAt string `json:"createdAt"`
}

// Store holds the admin account and the live sessions.
//
// Sessions are in memory on purpose. Restarting the relay logs everyone out,
// which is the right default for a panel whose buttons hand out shell access:
// a stolen session cookie should not outlive the process it was stolen from.
type Store struct {
	path string

	mu       sync.Mutex
	creds    Credentials
	sessions map[string]time.Time
}

// OpenStore loads the admin account at path. A missing file is not an error: it
// is a relay that has not been set up yet.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, sessions: map[string]time.Time{}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, &s.creds); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.creds.Username == "" || s.creds.Hash == "" {
		return nil, fmt.Errorf("%s: incomplete admin account", path)
	}
	return s, nil
}

// Configured reports whether setup has run.
func (s *Store) Configured() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.Username != ""
}

// Username is the configured admin name, for display.
func (s *Store) Username() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds.Username
}

// Setup creates the single admin account. It fails if one already exists, so the
// setup endpoint cannot be used to seize a running relay.
func (s *Store) Setup(username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("admin: username is required")
	}
	if len(password) < 8 {
		return errors.New("admin: password must be at least 8 characters")
	}
	hash, err := passhash.Hash(password)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.creds.Username != "" {
		return ErrConfigured
	}
	next := Credentials{Username: username, Hash: hash, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := s.write(next); err != nil {
		return err
	}
	s.creds = next
	return nil
}

// Verify checks a login attempt. It always does the full hash comparison, even
// for an unknown username, so response time does not reveal which names exist.
func (s *Store) Verify(username, password string) bool {
	s.mu.Lock()
	creds := s.creds
	s.mu.Unlock()
	if creds.Username == "" {
		return false
	}
	okUser := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(username)), []byte(creds.Username)) == 1
	okPass := passhash.Verify(creds.Hash, password)
	return okUser && okPass
}

// ChangePassword rotates the admin password, requiring the current one.
func (s *Store) ChangePassword(old, next string) error {
	if len(next) < 8 {
		return errors.New("admin: password must be at least 8 characters")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !passhash.Verify(s.creds.Hash, old) {
		return errors.New("admin: current password is wrong")
	}
	hash, err := passhash.Hash(next)
	if err != nil {
		return err
	}
	updated := s.creds
	updated.Hash = hash
	if err := s.write(updated); err != nil {
		return err
	}
	s.creds = updated
	// Every other session was minted against the old password; drop them.
	s.sessions = map[string]time.Time{}
	return nil
}

// NewSession mints a session token valid for ttl.
func (s *Store) NewSession(ttl time.Duration) (string, error) {
	tok, err := randomToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweepLocked()
	s.sessions[tok] = time.Now().Add(ttl)
	return tok, nil
}

// Session reports whether a session token is live.
func (s *Store) Session(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(s.sessions, token)
		return false
	}
	return true
}

// DropSession ends one session, for logout.
func (s *Store) DropSession(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *Store) sweepLocked() {
	now := time.Now()
	for tok, expiry := range s.sessions {
		if now.After(expiry) {
			delete(s.sessions, tok)
		}
	}
}

func (s *Store) write(c Credentials) error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// hashPassword and verifyPassword live in internal/passhash, shared with the
// registered-user accounts so the two credential files cannot drift apart.

// randomToken mints a session token. 32 bytes: it is a bearer credential for the
// panel that hands out shell access, so guessing it must be infeasible.
func randomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
