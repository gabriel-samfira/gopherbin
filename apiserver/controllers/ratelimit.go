package controllers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Login brute-force defense parameters: at most loginMaxFailures failed
// authentications per (clientIP, username) within a sliding window of
// loginWindowLength. While over the limit, further attempts are rejected
// without touching the user manager.
const (
	loginMaxFailures  = 10
	loginWindowLength = 15 * time.Minute
	// loginEvictThreshold triggers a lazy eviction sweep of the key map:
	// once it grows past this many entries, keys whose failure windows have
	// fully expired are dropped.
	loginEvictThreshold = 10000
)

// loginRateLimiter is a small in-memory sliding-window limiter used to slow
// down password brute-forcing on the login endpoint.
//
// The limiter is deliberately process-local: gopherbin is a single-binary
// application, so keeping the counters in a mutex-guarded map inside the API
// server process is the design trade-off (no external store). A restart
// resets the counters, and running several instances behind a load balancer
// multiplies the effective budget by the instance count.
//
// The clock is injectable (now) so tests can move time without sleeping.
type loginRateLimiter struct {
	mu sync.Mutex
	// now returns the current time; time.Now in production.
	now func() time.Time
	// failures maps a key to the timestamps of its failed attempts inside
	// the window, oldest first (appends only, pruned lazily).
	failures map[loginKey][]time.Time
}

// loginKey identifies one rate-limit bucket: client address plus the
// lowercased username.
type loginKey struct {
	ip       string
	username string
}

// newLoginRateLimiter returns a ready limiter. A nil now defaults to time.Now.
func newLoginRateLimiter(now func() time.Time) *loginRateLimiter {
	if now == nil {
		now = time.Now
	}
	return &loginRateLimiter{
		now:      now,
		failures: make(map[loginKey][]time.Time),
	}
}

// loginAttemptKey builds the limiter key for a login request.
func loginAttemptKey(r *http.Request, username string) loginKey {
	return loginKey{ip: clientIP(r), username: strings.ToLower(username)}
}

// allow reports whether an attempt may proceed, pruning expired timestamps
// for the key on the way.
func (l *loginRateLimiter) allow(key loginKey) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key, l.now())) < loginMaxFailures
}

// retryAfter returns the whole seconds until the oldest recorded failure
// leaves the window (i.e. until one slot frees up). It returns 0 when the
// key is not currently blocked.
func (l *loginRateLimiter) retryAfter(key loginKey) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	kept := l.prune(key, now)
	if len(kept) < loginMaxFailures {
		return 0
	}
	d := kept[0].Add(loginWindowLength).Sub(now)
	secs := int((d + time.Second - time.Nanosecond) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return secs
}

// recordFailure notes one failed authentication for the key and triggers a
// lazy eviction sweep of the whole map when it grows too large.
func (l *loginRateLimiter) recordFailure(key loginKey) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.prune(key, now)
	l.failures[key] = append(l.failures[key], now)
	if len(l.failures) > loginEvictThreshold {
		l.evict(now)
	}
}

// recordSuccess clears the failure history for the key: successes never
// count against the limit, and a valid login resets the bucket.
func (l *loginRateLimiter) recordSuccess(key loginKey) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// prune drops timestamps that fell out of the window and deletes the key when
// nothing remains. Callers must hold l.mu. It returns the live timestamps.
func (l *loginRateLimiter) prune(key loginKey, now time.Time) []time.Time {
	stamps, ok := l.failures[key]
	if !ok {
		return nil
	}
	cutoff := now.Add(-loginWindowLength)
	kept := stamps[:0]
	for _, s := range stamps {
		if s.After(cutoff) {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}

// evict sweeps every key and drops those whose windows have fully expired.
// Deleting from a map during range is safe in Go. Callers must hold l.mu.
func (l *loginRateLimiter) evict(now time.Time) {
	for key := range l.failures {
		l.prune(key, now)
	}
}

// clientIP extracts the transport-level peer address (r.RemoteAddr, minus the
// port). Note that http.Request carries no other peer-address field.
//
// It deliberately does NOT consult X-Forwarded-For or any other
// proxy-supplied header: without a configured, trusted proxy hop those
// headers are fully attacker-controlled and would let a brute-forcer rotate
// keys at will. As a documented consequence, when gopherbin is deployed
// behind a reverse proxy all clients share the proxy's address and therefore
// share one budget per username.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
