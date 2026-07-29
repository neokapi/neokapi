package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnonymousProjectRateLimit proves the unauthenticated anonymous-project
// route is throttled per client IP: once the burst is spent, further requests
// from the same IP get 429, and a different IP is unaffected.
func TestAnonymousProjectRateLimit(t *testing.T) {
	// Tight, deterministic limit for the test (read in SetupRoutes).
	t.Setenv("BOWRAIN_RL_ANON_PER_MIN", "1")
	t.Setenv("BOWRAIN_RL_ANON_BURST", "2")

	s, _ := newTestServer(t)
	e := s.GetEcho()

	post := func(ip string) int {
		body := `{"name":"Anon","default_source_language":"en"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/anonymous", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":40000"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	// First two (the burst) from IP A are admitted (Created), the third is 429.
	assert.Equal(t, http.StatusCreated, post("203.0.113.10"))
	assert.Equal(t, http.StatusCreated, post("203.0.113.10"))
	assert.Equal(t, http.StatusTooManyRequests, post("203.0.113.10"))

	// A different client IP has its own bucket and is not throttled.
	assert.Equal(t, http.StatusCreated, post("203.0.113.11"))
}

// TestPerIPControlsResistXFFSpoofing proves that once the server configures its
// echo.IPExtractor (configureIPExtractor, called from SetupRoutes), the per-IP
// controls key on the address the trusted reverse proxy appended (the right-most
// X-Forwarded-For hop) and CANNOT be bypassed by spoofing extra leftmost XFF
// hops. Without a configured IPExtractor echo trusts the leftmost XFF value, so
// a rotating spoof would dodge every per-IP limit and the /metrics private-IP
// gate — the hole this closes.
func TestPerIPControlsResistXFFSpoofing(t *testing.T) {
	// Read at GetEcho()/SetupRoutes time, so set before building the echo.
	t.Setenv("BOWRAIN_RL_ANON_PER_MIN", "1")
	t.Setenv("BOWRAIN_RL_ANON_BURST", "2")
	t.Setenv("BOWRAIN_METRICS_TOKEN", "") // exercise the private-IP gate branch

	s, _ := newTestServer(t)
	e := s.GetEcho()

	const proxy = "10.0.0.7:5555"     // trusted (private) reverse-proxy connection
	const realClient = "203.0.113.55" // client IP the proxy appends as right-most XFF

	t.Run("rotating spoofed leftmost XFF does not dodge the per-IP rate limit", func(t *testing.T) {
		post := func(spoofLeftmost string) int {
			body := `{"name":"Anon","default_source_language":"en"}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/anonymous", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			// The trusted proxy appends the real client as the RIGHT-MOST hop; an
			// attacker may prepend arbitrary spoofed hops to the left.
			req.Header.Set("X-Forwarded-For", spoofLeftmost+", "+realClient)
			req.RemoteAddr = proxy
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			return rec.Code
		}
		// All three key on realClient despite the rotating leftmost spoof: burst
		// of 2 admitted, third throttled.
		assert.Equal(t, http.StatusCreated, post("1.2.3.4"))
		assert.Equal(t, http.StatusCreated, post("5.6.7.8"))
		assert.Equal(t, http.StatusTooManyRequests, post("9.10.11.12"),
			"a rotating spoofed leftmost X-Forwarded-For must not create fresh per-IP buckets")
	})

	t.Run("spoofed X-Forwarded-For cannot pass the /metrics private-IP gate", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		// Direct connection is a routable public IP (server reachable via ingress);
		// attacker spoofs a loopback XFF to look internal.
		req.RemoteAddr = "203.0.113.200:44444"
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code,
			"a spoofed loopback X-Forwarded-For must not satisfy the /metrics internal-IP gate")
	})
}

// TestRateLimitByIPSharedBucket proves that passing the same RateLimitByIP
// middleware value to several routes makes them share one per-IP bucket, and
// that distinct IPs are independent.
func TestRateLimitByIPSharedBucket(t *testing.T) {
	mw := RateLimitByIP(1, 2) // 2 burst, 1/min sustained
	ok := func(c echo.Context) error { return c.NoContent(http.StatusOK) }
	e := echo.New()
	e.GET("/a", ok, mw)
	e.GET("/b", ok, mw)

	call := func(path, ip string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = ip + ":50000"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}

	// Two different routes drawing from the same shared bucket: A then B consume
	// the burst of 2, the third (either route) is 429.
	assert.Equal(t, http.StatusOK, call("/a", "198.51.100.5"))
	assert.Equal(t, http.StatusOK, call("/b", "198.51.100.5"))
	assert.Equal(t, http.StatusTooManyRequests, call("/a", "198.51.100.5"))

	// Retry-After hint is present on the throttled response.
	req := httptest.NewRequest(http.MethodGet, "/a", nil)
	req.RemoteAddr = "198.51.100.5:50000"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))

	// A distinct IP is unaffected.
	assert.Equal(t, http.StatusOK, call("/a", "198.51.100.6"))
}

// TestDeviceFlowRateLimit proves every leg of the device flow draws on the
// shared pre-auth per-IP bucket. Start alone was throttled once; verify and
// poll are the two calls that turn a user code into a session, so leaving
// either unmetered leaves the flow open to being hammered.
func TestDeviceFlowRateLimit(t *testing.T) {
	// Tight, deterministic limit for the test (read in SetupRoutes).
	t.Setenv("BOWRAIN_RL_AUTH_PER_MIN", "1")
	t.Setenv("BOWRAIN_RL_AUTH_BURST", "2")

	s, _ := newTestServer(t)
	e := s.GetEcho()

	send := func(method, path, body, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = ip + ":40000"
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}
	call := func(method, path, body, ip string) int {
		return send(method, path, body, ip).Code
	}

	// Each case uses its own client IP so the per-IP buckets do not interfere.
	for _, tc := range []struct {
		name, method, path, body, ip string
	}{
		{"POST verify", http.MethodPost, "/api/v1/auth/device/verify", "user_code=0000-0000-0000-0000", "203.0.113.21"},
		{"GET verify", http.MethodGet, "/api/v1/auth/device/verify?user_code=0000-0000-0000-0000", "", "203.0.113.22"},
		{"POST poll", http.MethodPost, "/api/v1/auth/device/poll", "device_code=nope", "203.0.113.23"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEqual(t, http.StatusTooManyRequests, call(tc.method, tc.path, tc.body, tc.ip))
			assert.NotEqual(t, http.StatusTooManyRequests, call(tc.method, tc.path, tc.body, tc.ip))
			assert.Equal(t, http.StatusTooManyRequests, call(tc.method, tc.path, tc.body, tc.ip),
				"the request past the burst must be throttled")
		})
	}

	// A throttled poll must answer in the device flow's own vocabulary. The CLI
	// polls in a loop and reads any error code it does not recognise as fatal,
	// so a generic throttle error would end a sign-in rather than pace it;
	// RFC 8628's slow_down is what tells the client to keep waiting.
	t.Run("a throttled poll says slow_down, not a fatal error", func(t *testing.T) {
		const ip = "203.0.113.31"
		rec := send(http.MethodPost, "/api/v1/auth/device/poll", "device_code=nope", ip)
		for range 5 {
			if rec.Code == http.StatusTooManyRequests {
				break
			}
			rec = send(http.MethodPost, "/api/v1/auth/device/poll", "device_code=nope", ip)
		}
		require.Equal(t, http.StatusTooManyRequests, rec.Code)

		var body struct {
			Error string `json:"error"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "slow_down", body.Error,
			"the device client keeps polling on slow_down and gives up on anything else")
		assert.Equal(t, "5", rec.Header().Get("Retry-After"))
	})

	// The device legs still draw on the SHARED pre-auth bucket: spending it on
	// /device/start throttles the poll from the same client.
	t.Run("start and poll share one bucket", func(t *testing.T) {
		const ip = "203.0.113.32"
		require.NotEqual(t, http.StatusTooManyRequests,
			call(http.MethodPost, "/api/v1/auth/device/start", "client_id=x", ip))
		require.NotEqual(t, http.StatusTooManyRequests,
			call(http.MethodPost, "/api/v1/auth/device/start", "client_id=x", ip))
		assert.Equal(t, http.StatusTooManyRequests,
			call(http.MethodPost, "/api/v1/auth/device/poll", "device_code=nope", ip),
			"the poll draws on the same per-IP bucket the starts just spent")
	})
}

// TestHourlyIPLimiter proves the imperative hourly per-IP limiter admits up to
// the burst and then denies, per IP, and treats an empty IP as allowed.
func TestHourlyIPLimiter(t *testing.T) {
	l := newHourlyIPLimiter(2) // burst 2, ~2/hour sustained

	assert.True(t, l.Allow("192.0.2.1"))
	assert.True(t, l.Allow("192.0.2.1"))
	assert.False(t, l.Allow("192.0.2.1"), "third within the hour is denied")

	// Separate IP has its own budget.
	assert.True(t, l.Allow("192.0.2.2"))

	// Empty IP cannot be keyed — never blocked.
	assert.True(t, l.Allow(""))
	assert.True(t, l.Allow(""))
	assert.True(t, l.Allow(""))
}
