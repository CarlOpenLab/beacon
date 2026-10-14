package relay

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// authLimiter slows credential guessing from a single source.
//
// The client token is the only thing between the open internet and a shell on
// every enrolled machine, and it is a value a person can remember. Without a
// limit, that is a guessing game with unlimited attempts; with one, guessing is
// pointless long before it is lucky.
//
// Counting is per source address and in-memory: a relay restart forgets, which
// is the right trade for a single-process service. A distributed attacker
// spreads across addresses and is not stopped by this — that is what a real
// token is for.
type authLimiter struct {
	mu         sync.Mutex
	now        func() time.Time
	max        int
	window     time.Duration
	cooldown   time.Duration
	trustProxy bool
	state      map[string]*attempts
}

type attempts struct {
	count int
	first time.Time
	until time.Time
}

func newAuthLimiter(max int, window, cooldown time.Duration, trustProxy bool) *authLimiter {
	return &authLimiter{
		now:        time.Now,
		max:        max,
		window:     window,
		cooldown:   cooldown,
		trustProxy: trustProxy,
		state:      map[string]*attempts{},
	}
}

// allow reports whether this source may try, and for how long it must wait if
// it may not.
func (l *authLimiter) allow(ip string) (bool, time.Duration) {
	if l == nil || l.max <= 0 {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked()

	a := l.state[ip]
	if a == nil {
		return true, 0
	}
	now := l.now()
	if now.Before(a.until) {
		return false, a.until.Sub(now)
	}
	// The cooldown has passed, and a stale window means the count starts over.
	if now.Sub(a.first) > l.window {
		delete(l.state, ip)
	}
	return true, 0
}

// fail records a rejected attempt and starts a cooldown once the source has
// used up its window.
func (l *authLimiter) fail(ip string) {
	if l == nil || l.max <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	a := l.state[ip]
	if a == nil || now.Sub(a.first) > l.window {
		a = &attempts{first: now}
		l.state[ip] = a
	}
	a.count++
	if a.count >= l.max {
		a.until = now.Add(l.cooldown)
		a.count = 0
		a.first = now
	}
}

// ok clears the record after a correct credential, so a legitimate client that
// fat-fingers once is not penalised later.
func (l *authLimiter) ok(ip string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	delete(l.state, ip)
	l.mu.Unlock()
}

// pruneLocked drops entries that can no longer matter, so a long-running relay
// does not accumulate one record per address that ever connected.
func (l *authLimiter) pruneLocked() {
	if len(l.state) < 4096 {
		return
	}
	now := l.now()
	for ip, a := range l.state {
		if now.After(a.until) && now.Sub(a.first) > l.window {
			delete(l.state, ip)
		}
	}
}

// clientIP returns the address to attribute a request to.
//
// X-Forwarded-For is only believed when the operator says a proxy is in front,
// because otherwise any caller could forge it and dodge the limiter by
// inventing a new address per attempt.
//
// When it is trusted, the *last* hop is used, not the first. A reverse proxy
// appends the address it actually saw, so with one proxy the list reads
// "<whatever the client claimed>, <the real client>". Taking the first element
// would trust exactly the value an attacker controls. This assumes a single
// trusted proxy; behind a chain, the last hop is the nearest proxy instead.
func clientIP(rq *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := rq.Header.Get("X-Forwarded-For"); xff != "" {
			hops := strings.Split(xff, ",")
			if last := strings.TrimSpace(hops[len(hops)-1]); last != "" {
				return last
			}
		}
	}
	host, _, err := net.SplitHostPort(rq.RemoteAddr)
	if err != nil {
		return rq.RemoteAddr
	}
	return host
}
