package httpcontrol

import (
	"crypto/sha256"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	controlTokenHeader  = "X-Control-Token"
	authRateLimitWindow = time.Minute
	authRateLimitMax    = 20
)

type controlPlaneAuthn struct {
	tokenHash [32]byte
	limiter   *ipRateLimiter
	required  bool
}

func newControlPlaneAuthn(token string) *controlPlaneAuthn {
	trimmed := strings.TrimSpace(token)

	authn := new(controlPlaneAuthn{
		required: trimmed != "",
		limiter:  newIPRateLimiter(authRateLimitWindow, authRateLimitMax),
	})
	if trimmed != "" {
		authn.tokenHash = sha256.Sum256([]byte(trimmed))
	}

	return authn
}

func (a *controlPlaneAuthn) authorize(r *http.Request) bool {
	if a == nil || !a.required {
		return true
	}

	provided := strings.TrimSpace(r.Header.Get(controlTokenHeader))
	if provided == "" {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
			provided = strings.TrimSpace(after)
		}
	}

	if provided == "" {
		return false
	}

	sum := sha256.Sum256([]byte(provided))

	return subtle.ConstantTimeCompare(sum[:], a.tokenHash[:]) == 1
}

func (a *controlPlaneAuthn) allowAuthAttempt(r *http.Request) bool {
	if a == nil || a.limiter == nil {
		return true
	}

	return a.limiter.allow(clientIP(r))
}

type ipRateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	maxHits int
	hits    map[string][]time.Time
	now     func() time.Time
}

func newIPRateLimiter(window time.Duration, maxHits int) *ipRateLimiter {
	return new(ipRateLimiter{
		window:  window,
		maxHits: maxHits,
		hits:    make(map[string][]time.Time),
		now:     time.Now,
	})
}

func (l *ipRateLimiter) allow(ip string) bool {
	if ip == "" {
		ip = "unknown"
	}

	now := l.now()
	cutoff := now.Add(-l.window)
	l.mu.Lock()
	defer l.mu.Unlock()

	times := l.hits[ip]

	kept := times[:0]
	for _, ts := range times {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}

	if len(kept) >= l.maxHits {
		l.hits[ip] = kept

		return false
	}

	l.hits[ip] = append(kept, now)

	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}

	return host
}

func requireControlToken(authn *controlPlaneAuthn, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authn.authorize(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				jsonKeyStatus: statusUnauthorized,
				jsonKeyError:  "missing or invalid control plane token",
			})

			return
		}

		next(w, r)
	}
}
