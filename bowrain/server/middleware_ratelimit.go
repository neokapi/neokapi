package server

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

// projectRateLimiter provides per-project rate limiting.
type projectRateLimiter struct {
	mu        sync.Mutex
	limiters  map[string]*rate.Limiter
	rate      rate.Limit // requests per second
	burst     int
	lastReset time.Time
}

func newProjectRateLimiter(r rate.Limit, burst int) *projectRateLimiter {
	return &projectRateLimiter{
		limiters:  make(map[string]*rate.Limiter),
		rate:      r,
		burst:     burst,
		lastReset: time.Now(),
	}
}

func (p *projectRateLimiter) getLimiter(projectID string) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Lazy cleanup to prevent memory growth: reset the map every hour.
	// Limiters are cheap to recreate, and doing it inline avoids a
	// background goroutine that would outlive the server.
	if time.Since(p.lastReset) > 1*time.Hour {
		p.limiters = make(map[string]*rate.Limiter)
		p.lastReset = time.Now()
	}

	l, ok := p.limiters[projectID]
	if !ok {
		l = rate.NewLimiter(p.rate, p.burst)
		p.limiters[projectID] = l
	}
	return l
}

// RateLimitSyncPush returns middleware that rate-limits sync push requests per project.
// Allows `burst` requests immediately, then `perMinute` per minute sustained.
func RateLimitSyncPush(perMinute int, burst int) echo.MiddlewareFunc {
	limiter := newProjectRateLimiter(rate.Limit(float64(perMinute)/60.0), burst)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			projectID := c.Param("id")
			if projectID == "" {
				return next(c)
			}
			if !limiter.getLimiter(projectID).Allow() {
				return c.JSON(http.StatusTooManyRequests, ErrorResponse{
					Error: "rate limit exceeded: max " + c.QueryParam("_rl_info") + " pushes per minute per project",
				})
			}
			return next(c)
		}
	}
}

// RateLimitByIP returns middleware that throttles requests per client IP,
// allowing `burst` immediately and then `perMinute` per minute sustained. The
// key is c.RealIP() — the same client-IP notion used for audit attribution
// (requestMeta). RealIP honors X-Forwarded-For / X-Real-IP, so it is only as
// trustworthy as the reverse proxy in front of the server; in Bowrain's
// deployment the ingress sets those headers and the server is not directly
// exposed. On saturation it responds 429 with a Retry-After hint.
//
// A single RateLimitByIP value can be shared across several routes (pass the
// same middleware to each) so they draw from one per-IP bucket; passing a fresh
// value per route gives each route an independent bucket.
func RateLimitByIP(perMinute, burst int) echo.MiddlewareFunc {
	return newIPThrottle(perMinute, burst).middleware(nil)
}

// ipThrottle is a per-client-IP token bucket that several routes share. It
// exists so routes can draw on ONE bucket while still answering a throttled
// request in their own terms: a protocol that defines its own back-off signal
// must be able to send it, because a client that reads a generic error as fatal
// will abandon work the throttle only meant to slow down.
type ipThrottle struct {
	*projectRateLimiter
}

func newIPThrottle(perMinute, burst int) *ipThrottle {
	return &ipThrottle{newProjectRateLimiter(rate.Limit(float64(perMinute)/60.0), burst)}
}

// middleware returns Echo middleware drawing on this throttle. deny renders the
// throttled response; nil gives the generic 429 with a Retry-After hint.
func (t *ipThrottle) middleware(deny echo.HandlerFunc) echo.MiddlewareFunc {
	if deny == nil {
		deny = func(c echo.Context) error {
			c.Response().Header().Set("Retry-After", "60")
			return c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error: "rate limit exceeded: too many requests from this client, retry later",
			})
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()
			if ip == "" {
				return next(c) // cannot key — do not block
			}
			if !t.getLimiter(ip).Allow() {
				return deny(c)
			}
			return next(c)
		}
	}
}

// hourlyIPLimiter is a per-client-IP limiter with an hourly window, used for
// low-frequency, abuse-prone actions that fall inside a handler rather than a
// dedicated route (e.g. claim-email sends triggered only when an email is
// supplied to anonymous project creation). Unlike RateLimitByIP it is consulted
// imperatively via Allow so the handler can decide what to do when the cap is
// hit (here: create the project but skip the email).
type hourlyIPLimiter struct {
	*projectRateLimiter
}

// newHourlyIPLimiter caps actions to `perHour` per client IP, allowing a small
// burst equal to perHour (or at least 1) so the first few in an hour are not
// artificially delayed.
func newHourlyIPLimiter(perHour int) *hourlyIPLimiter {
	burst := max(perHour, 1)
	return &hourlyIPLimiter{newProjectRateLimiter(rate.Limit(float64(perHour)/3600.0), burst)}
}

// Allow reports whether an action from ip is within the hourly budget.
func (h *hourlyIPLimiter) Allow(ip string) bool {
	if ip == "" {
		return true
	}
	return h.getLimiter(ip).Allow()
}

// rateLimitDefaults holds the env-overridable per-IP throttle knobs. All are
// requests-per-minute except ClaimEmailPerHour (an hourly cap on outbound
// claim emails). Each field is overridable via a BOWRAIN_RL_* environment
// variable; unset/invalid values fall back to the compiled defaults.
type rateLimitDefaults struct {
	AnonPerMin        int
	AnonBurst         int
	ClaimEmailPerHour int
	AuthPerMin        int
	AuthBurst         int
	InvitePerMin      int
	InviteBurst       int
	AIPerMin          int
	AIBurst           int
}

// loadRateLimitDefaults reads the BOWRAIN_RL_* environment overrides over a set
// of conservative defaults. Anonymous project creation and auth endpoints are
// unauthenticated (or pre-auth) and thus the most abuse-prone, so they get the
// tightest caps.
func loadRateLimitDefaults() rateLimitDefaults {
	return rateLimitDefaults{
		AnonPerMin:        rlEnvInt("BOWRAIN_RL_ANON_PER_MIN", 10),
		AnonBurst:         rlEnvInt("BOWRAIN_RL_ANON_BURST", 5),
		ClaimEmailPerHour: rlEnvInt("BOWRAIN_RL_CLAIM_EMAIL_PER_HOUR", 5),
		AuthPerMin:        rlEnvInt("BOWRAIN_RL_AUTH_PER_MIN", 30),
		AuthBurst:         rlEnvInt("BOWRAIN_RL_AUTH_BURST", 15),
		InvitePerMin:      rlEnvInt("BOWRAIN_RL_INVITE_PER_MIN", 20),
		InviteBurst:       rlEnvInt("BOWRAIN_RL_INVITE_BURST", 10),
		AIPerMin:          rlEnvInt("BOWRAIN_RL_AI_PER_MIN", 20),
		AIBurst:           rlEnvInt("BOWRAIN_RL_AI_BURST", 10),
	}
}

// rlEnvInt reads a positive integer from the named environment variable,
// falling back to def when unset, unparsable, or non-positive.
func rlEnvInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
