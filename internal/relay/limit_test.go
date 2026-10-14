package relay

import (
	"net/http"
	"testing"
	"time"
)

func at(clock *time.Time) func() time.Time {
	return func() time.Time { return *clock }
}

func TestLimiterLocksOutAfterRepeatedFailures(t *testing.T) {
	now := time.Unix(0, 0)
	l := newAuthLimiter(3, time.Minute, 5*time.Minute, false)
	l.now = at(&now)

	for i := 0; i < 2; i++ {
		if ok, _ := l.allow("1.2.3.4"); !ok {
			t.Fatalf("attempt %d was refused before the limit", i+1)
		}
		l.fail("1.2.3.4")
	}
	// Third failure trips the lockout.
	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Fatal("third attempt should still be allowed")
	}
	l.fail("1.2.3.4")

	ok, wait := l.allow("1.2.3.4")
	if ok {
		t.Fatal("a source past the limit was allowed again")
	}
	if wait <= 0 || wait > 5*time.Minute {
		t.Fatalf("wait = %v, want a positive wait within the cooldown", wait)
	}

	// A different source is unaffected: the limiter is per address, not global.
	if ok, _ := l.allow("5.6.7.8"); !ok {
		t.Fatal("an unrelated source was locked out")
	}

	now = now.Add(6 * time.Minute)
	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Fatal("the lockout outlived its cooldown")
	}
}

func TestLimiterSuccessClearsFailures(t *testing.T) {
	now := time.Unix(0, 0)
	l := newAuthLimiter(3, time.Minute, 5*time.Minute, false)
	l.now = at(&now)

	// Two typos, then a correct credential, then two more: without the reset
	// this would lock out a legitimate user who mistyped twice.
	l.fail("1.2.3.4")
	l.fail("1.2.3.4")
	l.ok("1.2.3.4")
	l.fail("1.2.3.4")
	l.fail("1.2.3.4")

	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Fatal("a correct credential did not reset the failure count")
	}
}

func TestLimiterDisabled(t *testing.T) {
	now := time.Unix(0, 0)
	l := newAuthLimiter(0, time.Minute, time.Minute, false)
	l.now = at(&now)
	for i := 0; i < 100; i++ {
		l.fail("1.2.3.4")
	}
	if ok, _ := l.allow("1.2.3.4"); !ok {
		t.Fatal("a limiter with max 0 must never refuse")
	}
}

func TestClientIP(t *testing.T) {
	rq := &http.Request{RemoteAddr: "10.0.0.9:51234", Header: http.Header{}}

	// The shape a proxy produces: whatever the caller claimed, then the address
	// the proxy actually saw.
	rq.Header.Set("X-Forwarded-For", "1.1.1.1, 203.0.113.7")
	// Not trusted by default: a caller could otherwise invent an address per
	// attempt and dodge the limiter entirely.
	if got := clientIP(rq, false); got != "10.0.0.9" {
		t.Fatalf("untrusted proxy: got %q, want the socket address", got)
	}
	if got := clientIP(rq, true); got != "203.0.113.7" {
		t.Fatalf("trusted proxy: got %q, want the hop the proxy appended", got)
	}

	rq.Header.Del("X-Forwarded-For")
	if got := clientIP(rq, true); got != "10.0.0.9" {
		t.Fatalf("no header: got %q, want the socket address", got)
	}

	// A RemoteAddr with no port must not be mangled by SplitHostPort.
	rq.RemoteAddr = "weird"
	if got := clientIP(rq, false); got != "weird" {
		t.Fatalf("portless address: got %q, want it unchanged", got)
	}
}
