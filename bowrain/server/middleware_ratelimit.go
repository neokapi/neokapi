package server

import (
	"net/http"
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
