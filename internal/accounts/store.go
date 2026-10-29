// Package accounts is the registered-user side of a multi-tenant relay: an
// open signup, an approval gate held by the super admin, and the login token
// that reaches exactly the machines enrolled under the user's tenant.
//
// It is a file store, like everything else in the relay: one users.json beside
// the device registry, written atomically, holding the users and the HMAC key
// that signs their login tokens. A relay with a thousand approved users still
// fits in a JSON file, and one that cannot fit in a JSON file has outgrown
// single-binary deployments anyway.
package accounts

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CarlOpenLab/beacon/internal/passhash"
)

// User statuses. A user is created pending, and only active users hold working
// login tokens — approving or rejecting bumps the user's token floor, so a
// token minted before the decision dies with the old status.
const (
	StatusPending  = "pending"
	StatusActive   = "active"
	StatusRejected = "rejected"
)

// DefaultTokenTTL is how long a login token lives when the caller does not
// choose. Long enough that a person is not re-typing a password daily, short
// enough that a leaked token is not forever.
const DefaultTokenTTL = 7 * 24 * time.Hour

// Errors reported to operators and callers. They are compared with errors.Is.
var (
	ErrTaken          = errors.New("accounts: that username is already registered")
	ErrBadUsername    = errors.New("accounts: username must be 3-32 characters of letters, digits, dot, dash or underscore")
	ErrBadPassword    = errors.New("accounts: password must be at least 8 characters")
	ErrBadCredentials = errors.New("accounts: wrong username or password")
	ErrPending        = errors.New("accounts: this account is waiting for approval")
	ErrRejected       = errors.New("accounts: this account was not approved")
	ErrNotFound       = errors.New("accounts: no such user")
	ErrFull           = errors.New("accounts: this relay is not accepting more registrations")
)

// maxUsers bounds users.json growth: signup is open to the internet, and an
// unbounded file rewritten on every POST is a slow crash.
const maxUsers = 10_000

// User is one registered account. ID doubles as the tenant id: machines the
// user enrolls carry it, and the roster they are shown is filtered to it.
type User struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Hash       string `json:"hash"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	ApprovedAt string `json:"approvedAt,omitempty"`
	// NotBefore is a unix-seconds floor on login tokens: minting sets it to the
	// approval moment, and approve/reject/delete bumps it, so a token handed
	// out under one status does not survive into the next.
	NotBefore int64 `json:"notBefore,omitempty"`
}

// usersFile is the on-disk shape. The signing key lives in the same file as the
// users: one less secret on disk, and a store copied to a new machine carries
// its tokens' verifiability with it.
type usersFile struct {
	Secret string           `json:"secret"`
	Users  map[string]*User `json:"users"`
}

// Store holds the registered users and signs their login tokens.
type Store struct {
	path string
	ttl  time.Duration

	mu   sync.Mutex
	file usersFile
}

// Open loads users.json, creating an empty store — with a fresh signing key —
// when the file does not exist yet.
func Open(path string, tokenTTL time.Duration) (*Store, error) {
	if tokenTTL <= 0 {
		tokenTTL = DefaultTokenTTL
	}
	s := &Store{path: path, ttl: tokenTTL, file: usersFile{Users: map[string]*User{}}}
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if err := s.initSecret(); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err := json.Unmarshal(b, &s.file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.file.Users == nil {
		s.file.Users = map[string]*User{}
	}
	if s.file.Secret == "" {
		if err := s.initSecret(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// initSecret mints the HMAC key and persists it. Called on a fresh file, or on
// an old one that predates tokens: a store that never signed a token cannot
// have a token to invalidate.
func (s *Store) initSecret() error {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	s.file.Secret = base64.RawURLEncoding.EncodeToString(key)
	return s.write()
}

// write persists the store atomically: temp file in the same directory, then
// rename. 0600 because the file signs login tokens.
func (s *Store) write() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.file, "", "  ")
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

// Path is where the store is persisted.
func (s *Store) Path() string { return s.path }

// Register creates a pending account. The name is unique case-insensitively:
// two people typing "Alice" and "alice" are one person arguing with a shift key.
func (s *Store) Register(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if !validUsername(username) {
		return nil, ErrBadUsername
	}
	if len(password) < 8 {
		return nil, ErrBadPassword
	}
	hash, err := passhash.Hash(password)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.file.Users) >= maxUsers {
		return nil, ErrFull
	}
	for _, u := range s.file.Users {
		if strings.EqualFold(u.Username, username) {
			return nil, ErrTaken
		}
	}
	u := &User{
		ID:        newUserID(),
		Username:  username,
		Hash:      hash,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	s.file.Users[u.ID] = u
	if err := s.write(); err != nil {
		delete(s.file.Users, u.ID)
		return nil, err
	}
	return u, nil
}

// Authenticate checks a login attempt and returns the user, whose Status the
// caller must honour (pending and rejected users get a specific refusal, not a
// generic one, so the person knows to wait rather than to retry).
func (s *Store) Authenticate(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	s.mu.Lock()
	defer s.mu.Unlock()
	var found *User
	for _, u := range s.file.Users {
		if strings.EqualFold(u.Username, username) {
			found = u
			break
		}
	}
	if found == nil {
		// Burn a hash comparison anyway, so a missing name and a wrong password
		// take the same time.
		passhash.Verify("pbkdf2_sha256$210000$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", password)
		return nil, ErrBadCredentials
	}
	if !passhash.Verify(found.Hash, password) {
		return nil, ErrBadCredentials
	}
	switch found.Status {
	case StatusActive:
		return found, nil
	case StatusPending:
		return nil, ErrPending
	default:
		return nil, ErrRejected
	}
}

// List returns every user, oldest first.
func (s *Store) List() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.file.Users))
	for _, u := range s.file.Users {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// ByID returns one user.
func (s *Store) ByID(id string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.file.Users[id]
	if !ok {
		return User{}, false
	}
	return *u, true
}

// Approve marks a pending or rejected user active and invalidates their older
// tokens: whatever a token was minted under, it now answers to "active".
func (s *Store) Approve(id string) error {
	return s.setStatus(id, StatusActive)
}

// Reject marks a user rejected and kills their tokens now.
func (s *Store) Reject(id string) error {
	return s.setStatus(id, StatusRejected)
}

func (s *Store) setStatus(id, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.file.Users[id]
	if !ok {
		return ErrNotFound
	}
	u.Status = status
	u.NotBefore = time.Now().Unix()
	if status == StatusActive {
		u.ApprovedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return s.write()
}

// Delete removes a user and kills their tokens. Their machines stay enrolled —
// the operator decides what happens to hardware — but every token minted under
// the tenant stops verifying, so the machines' roster and the user's reach end
// at once.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.file.Users[id]; !ok {
		return ErrNotFound
	}
	delete(s.file.Users, id)
	return s.write()
}

// TokenTTL is how long issued login tokens live.
func (s *Store) TokenTTL() time.Duration { return s.ttl }

// claims is the inside of a login token. It is signed, not encrypted: the
// username rides along in the clear, like the session it stands for.
type claims struct {
	User   string `json:"u"`
	Tenant string `json:"t"`
	Issued int64  `json:"i"`
	Expiry int64  `json:"e"`
}

// LoginToken mints a signed bearer token for an active user. Stateless on
// purpose: verifying one is an HMAC over an in-memory map, not a session table
// to sweep, and a restart does not sign anyone out of a console they were
// approved into.
func (s *Store) LoginToken(u *User) (string, time.Time, error) {
	if u == nil || u.Status != StatusActive {
		return "", time.Time{}, ErrBadCredentials
	}
	now := time.Now()
	c := claims{
		User:   u.Username,
		Tenant: u.ID,
		Issued: now.Unix(),
		Expiry: now.Add(s.ttl).Unix(),
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return "", time.Time{}, err
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig, err := s.sign(body)
	if err != nil {
		return "", time.Time{}, err
	}
	return body + "." + sig, now.Add(s.ttl), nil
}

// sign HMACs the token body with the store's key.
func (s *Store) sign(body string) (string, error) {
	key, err := base64.RawURLEncoding.DecodeString(s.file.Secret)
	if err != nil {
		return "", fmt.Errorf("accounts: signing key: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// VerifyLoginToken checks a bearer token and reports the tenant and username it
// stands for. It is the shape relay.TokenStore's LoginVerifier asks for, which
// is how a user token rides the same admission path as every other credential.
func (s *Store) VerifyLoginToken(token string) (tenant, user string, ok bool) {
	body, sig, found := strings.Cut(token, ".")
	if !found || body == "" || sig == "" {
		return "", "", false
	}
	want, err := s.sign(body)
	if err != nil || subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return "", "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", "", false
	}
	var c claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return "", "", false
	}
	if time.Now().Unix() >= c.Expiry || c.Issued > time.Now().Unix() {
		return "", "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Claims name the account by username; the store keys rows by id. Re-find
	// the row and re-check the id, so a token minted for one tenant can never
	// be re-presented as another's.
	var u *User
	for _, row := range s.file.Users {
		if strings.EqualFold(row.Username, c.User) {
			u = row
			break
		}
	}
	if u == nil || u.ID != c.Tenant || u.Status != StatusActive {
		return "", "", false
	}
	if c.Issued < u.NotBefore {
		return "", "", false
	}
	return c.Tenant, c.User, true
}

// validUsername keeps names that read the same in a devices.json, an admin
// table and a shell line.
func validUsername(name string) bool {
	if len(name) < 3 || len(name) > 32 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// newUserID mints a tenant id: short enough to read across a table, random
// enough that one user's ids mean nothing about another's.
func newUserID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// The clock is a poorer secret but a working id; registering also fails
		// loudly if the machine has lost its randomness.
		return fmt.Sprintf("u-%x", time.Now().UnixNano())
	}
	return "u-" + fmt.Sprintf("%x", b[:])
}
