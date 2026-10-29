package accounts

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A user's whole life: signup lands pending, login refuses, approval opens the
// door, and every later decision — reject, delete — takes the token away at
// once rather than at the next expiry.
func TestUserLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	s, err := Open(path, time.Hour)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	u, err := s.Register("alice", "hunter2hunter2")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.Status != StatusPending {
		t.Fatalf("status = %q, want pending", u.Status)
	}
	if _, err := s.Register("ALICE", "hunter2hunter2"); !errors.Is(err, ErrTaken) {
		t.Fatalf("case-insensitive duplicate = %v, want ErrTaken", err)
	}

	if _, err := s.Authenticate("alice", "hunter2hunter2"); !errors.Is(err, ErrPending) {
		t.Fatalf("login while pending = %v, want ErrPending", err)
	}
	if _, err := s.Authenticate("alice", "wrong-wrong-wrong"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("wrong password = %v, want ErrBadCredentials", err)
	}
	if _, err := s.Authenticate("nobody", "hunter2hunter2"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("unknown user = %v, want ErrBadCredentials", err)
	}

	if err := s.Approve(u.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}
	got, err := s.Authenticate("alice", "hunter2hunter2")
	if err != nil || got.Status != StatusActive {
		t.Fatalf("login after approval = (%v, %v), want active", got, err)
	}

	token, exp, err := s.LoginToken(got)
	if err != nil {
		t.Fatalf("login token: %v", err)
	}
	if !exp.After(time.Now()) {
		t.Fatalf("token already expired at %v", exp)
	}
	tenant, name, ok := s.VerifyLoginToken(token)
	if !ok || tenant != u.ID || name != "alice" {
		t.Fatalf("verify = (%q, %q, %v), want tenant %q and alice", tenant, name, ok, u.ID)
	}
	// A tampered body is a dead token, not a panic and not a pass.
	if _, _, ok := s.VerifyLoginToken(token + "x"); ok {
		t.Fatal("a tampered token verified")
	}
	if _, _, ok := s.VerifyLoginToken("nonsense"); ok {
		t.Fatal("garbage verified")
	}

	// The store survives a restart with its tokens verifiable: the key rides in
	// the same file as the users.
	reopened, err := Open(path, time.Hour)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, _, ok := reopened.VerifyLoginToken(token); !ok {
		t.Fatal("token did not survive a reopen")
	}

	// Rejection is immediate: the same token, minted before the decision, dies.
	if err := s.Reject(u.ID); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := s.Authenticate("alice", "hunter2hunter2"); !errors.Is(err, ErrRejected) {
		t.Fatalf("login after rejection = %v, want ErrRejected", err)
	}
	if _, _, ok := s.VerifyLoginToken(token); ok {
		t.Fatal("a token survived rejection")
	}

	// Approval again mints fresh reach; deletion ends it for good.
	if err := s.Approve(u.ID); err != nil {
		t.Fatalf("re-approve: %v", err)
	}
	got, err = s.Authenticate("alice", "hunter2hunter2")
	if err != nil {
		t.Fatalf("login after re-approval: %v", err)
	}
	token, _, err = s.LoginToken(got)
	if err != nil {
		t.Fatalf("login token: %v", err)
	}
	if err := s.Delete(u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, _, ok := s.VerifyLoginToken(token); ok {
		t.Fatal("a token survived deletion")
	}
}

// A store file is 0600 and valid JSON with the secret inside; the passwords are
// hashes, never the passwords.
func TestStoreFileShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.json")
	s, err := Open(path, time.Hour)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := s.Register("bob", "hunter2hunter2"); err != nil {
		t.Fatalf("register: %v", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Windows does not emulate the POSIX mode bits; the 0600 is honoured where
	// it means something.
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(b), "hunter2hunter2") {
		t.Fatal("the file leaks the password")
	}
}
