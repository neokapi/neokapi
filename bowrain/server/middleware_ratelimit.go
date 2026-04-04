package server

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"
)

// projectRateLimiter provides per-project rate limiting.
type projectRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit // requests per second
	burst    int
}

func newProjectRateLimiter(r rate.Limit, burst int) *projectRateLimiter {
	return &projectRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    burst,
	}
}

func (p *projectRateLimiter) getLimiter(projectID string) *rate.Limiter {
	p.mu.Lock()
	defer p.mu.Unlock()

	l, ok := p.limiters[projectID]
	if !ok {
		l = rate.NewLimiter(p.rate, p.burst)
		p.limiters[projectID] = l
	}
	return l
}

// cleanup removes stale limiters (call periodically).
func (p *projectRateLimiter) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Simple: reset map every hour. Limiters are cheap to recreate.
	p.limiters = make(map[string]*rate.Limiter)
}

// RateLimitSyncPush returns middleware that rate-limits sync push requests per project.
// Allows `burst` requests immediately, then `perMinute` per minute sustained.
func RateLimitSyncPush(perMinute int, burst int) echo.MiddlewareFunc {
	limiter := newProjectRateLimiter(rate.Limit(float64(perMinute)/60.0), burst)

	// Periodic cleanup to prevent memory growth.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in rate limiter cleanup", "panic", r)
			}
		}()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			limiter.cleanup()
		}
	}()

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
