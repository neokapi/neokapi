package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host/config"
)

// setBuildInfo swaps the ldflags-injected key/endpoint for the duration of a
// test, restoring the previous values afterwards.
func setBuildInfo(t *testing.T, key, ep string) {
	t.Helper()
	prevKey, prevEndpoint := apiKey, endpoint
	Configure(key, ep)
	t.Cleanup(func() { Configure(prevKey, prevEndpoint) })
}

// clearLatticeEnv neutralizes every environment switch in the opt-out
// lattice (the test runner itself often sets CI=true) and isolates the
// config store in a temp dir.
func clearLatticeEnv(t *testing.T) *config.AppConfig {
	t.Helper()
	t.Setenv(EnvOptOut, "")
	t.Setenv(EnvDoNotTrack, "")
	t.Setenv(EnvCI, "")
	t.Setenv("KAPI_TELEMETRY_ENABLED", "")
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	cfg := config.NewAppConfig()
	require.NoError(t, cfg.Load())
	return cfg
}

func TestResolveLattice(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		env         map[string]string
		configOff   bool
		wantEnabled bool
		wantReason  string
	}{
		{
			name:        "enabled when key present and no switch fires",
			key:         "phc_test",
			wantEnabled: true,
			wantReason:  ReasonEnabled,
		},
		{
			name:       "no baked-in key disables (dev build)",
			key:        "",
			wantReason: ReasonNoKey,
		},
		{
			name:       "persisted config off wins",
			key:        "phc_test",
			configOff:  true,
			wantReason: ReasonConfigOff,
		},
		{
			name:       "KAPI_TELEMETRY=0 disables",
			key:        "phc_test",
			env:        map[string]string{EnvOptOut: "0"},
			wantReason: ReasonEnvOptOut,
		},
		{
			name:       "KAPI_TELEMETRY=false disables",
			key:        "phc_test",
			env:        map[string]string{EnvOptOut: "false"},
			wantReason: ReasonEnvOptOut,
		},
		{
			name:        "KAPI_TELEMETRY=1 does not disable",
			key:         "phc_test",
			env:         map[string]string{EnvOptOut: "1"},
			wantEnabled: true,
			wantReason:  ReasonEnabled,
		},
		{
			name:       "DO_NOT_TRACK=1 disables",
			key:        "phc_test",
			env:        map[string]string{EnvDoNotTrack: "1"},
			wantReason: ReasonDoNotTrack,
		},
		{
			name:       "DO_NOT_TRACK any non-zero value disables",
			key:        "phc_test",
			env:        map[string]string{EnvDoNotTrack: "yes"},
			wantReason: ReasonDoNotTrack,
		},
		{
			name:        "DO_NOT_TRACK=0 does not disable",
			key:         "phc_test",
			env:         map[string]string{EnvDoNotTrack: "0"},
			wantEnabled: true,
			wantReason:  ReasonEnabled,
		},
		{
			name:       "CI=true disables",
			key:        "phc_test",
			env:        map[string]string{EnvCI: "true"},
			wantReason: ReasonCI,
		},
		{
			name:        "CI=false does not disable",
			key:         "phc_test",
			env:         map[string]string{EnvCI: "false"},
			wantEnabled: true,
			wantReason:  ReasonEnabled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := clearLatticeEnv(t)
			setBuildInfo(t, tt.key, "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if tt.configOff {
				cfg.Set(KeyEnabled, "false")
			}
			st := Resolve(cfg)
			assert.Equal(t, tt.wantEnabled, st.Enabled)
			assert.Equal(t, tt.wantReason, st.Reason)
		})
	}
}

func TestConfigOffValues(t *testing.T) {
	tests := []struct {
		value string
		off   bool
	}{
		{"", false},
		{"true", false},
		{"on", false},
		{"false", true},
		{"False", true},
		{"0", true},
		{"off", true},
		{"no", true},
	}
	for _, tt := range tests {
		t.Run("value="+tt.value, func(t *testing.T) {
			assert.Equal(t, tt.off, configOff(tt.value))
		})
	}
}

func TestMachineIDGeneratedOnceAndPersisted(t *testing.T) {
	cfg := clearLatticeEnv(t)

	id := MachineID(cfg)
	_, err := uuid.Parse(id)
	require.NoError(t, err, "machine id must be a random UUID")

	// Stable within the same config instance.
	assert.Equal(t, id, MachineID(cfg))

	// Stable across a fresh config load (persisted to the config store).
	fresh := config.NewAppConfig()
	require.NoError(t, fresh.Load())
	assert.Equal(t, id, MachineID(fresh))
}

func TestDurationBucket(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Millisecond, "<100ms"},
		{99 * time.Millisecond, "<100ms"},
		{100 * time.Millisecond, "<1s"},
		{999 * time.Millisecond, "<1s"},
		{time.Second, "<10s"},
		{9 * time.Second, "<10s"},
		{30 * time.Second, "<60s"},
		{time.Minute, ">=60s"},
		{time.Hour, ">=60s"},
	}
	for _, tt := range tests {
		t.Run(tt.want+"/"+tt.d.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, DurationBucket(tt.d))
		})
	}
}

func TestClientPostsBatch(t *testing.T) {
	cfg := clearLatticeEnv(t)

	type received struct {
		body []byte
	}
	got := make(chan received, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- received{body: body}
	}))
	defer srv.Close()
	setBuildInfo(t, "phc_test", srv.URL)

	c := New(cfg, Options{Surface: "cli", Version: "1.2.0-test"})
	require.NotNil(t, c)
	c.Capture("cli_command_run", map[string]any{
		"command":         "stats",
		"duration_bucket": "<1s",
		"exit_class":      "ok",
	})
	c.Close(time.Second)

	select {
	case r := <-got:
		var payload struct {
			APIKey string `json:"api_key"`
			Batch  []struct {
				Event      string         `json:"event"`
				DistinctID string         `json:"distinct_id"`
				Properties map[string]any `json:"properties"`
			} `json:"batch"`
		}
		require.NoError(t, json.Unmarshal(r.body, &payload))
		assert.Equal(t, "phc_test", payload.APIKey)
		require.Len(t, payload.Batch, 1)
		ev := payload.Batch[0]
		assert.Equal(t, "cli_command_run", ev.Event)
		_, err := uuid.Parse(ev.DistinctID)
		require.NoError(t, err, "distinct_id must be the anonymous machine UUID")
		assert.Equal(t, "stats", ev.Properties["command"])
		assert.Equal(t, "<1s", ev.Properties["duration_bucket"])
		assert.Equal(t, "ok", ev.Properties["exit_class"])
		assert.Equal(t, "cli", ev.Properties["surface"])
		assert.Equal(t, "1.2.0-test", ev.Properties["app_version"])
		assert.NotEmpty(t, ev.Properties["os"])
		assert.NotEmpty(t, ev.Properties["arch"])
	case <-time.After(5 * time.Second):
		t.Fatal("no batch received")
	}
}

func TestDisabledEnvAttemptsNoHTTP(t *testing.T) {
	cfg := clearLatticeEnv(t)

	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	}))
	defer srv.Close()
	setBuildInfo(t, "phc_test", srv.URL)
	t.Setenv(EnvOptOut, "0")

	c := New(cfg, Options{Surface: "cli", Version: "test"})
	assert.Nil(t, c, "KAPI_TELEMETRY=0 must yield a nil (inert) client")

	// The nil client is fully inert.
	c.Capture("cli_command_run", map[string]any{"command": "stats"})
	c.Close(time.Second)

	time.Sleep(50 * time.Millisecond)
	assert.Zero(t, calls.Load(), "no HTTP request may be attempted when opted out")
}

func TestCloseNeverBlocksPastWait(t *testing.T) {
	cfg := clearLatticeEnv(t)

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hang the request well past the flush budget
	}))
	defer srv.Close()
	defer close(release)
	setBuildInfo(t, "phc_test", srv.URL)

	c := New(cfg, Options{Surface: "cli", Version: "test"})
	require.NotNil(t, c)
	c.Capture("cli_first_run", nil)

	start := time.Now()
	c.Close(100 * time.Millisecond)
	assert.Less(t, time.Since(start), time.Second,
		"Close must return within the flush budget even when the endpoint hangs")

	// Idempotent and still non-blocking after close.
	c.Capture("cli_command_run", nil)
	c.Close(100 * time.Millisecond)
}

func TestCaptureNeverBlocksWhenQueueFull(t *testing.T) {
	cfg := clearLatticeEnv(t)

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)
	setBuildInfo(t, "phc_test", srv.URL)

	c := New(cfg, Options{Surface: "cli", Version: "test"})
	require.NotNil(t, c)

	start := time.Now()
	for range queueSize * 4 {
		c.Capture("cli_command_run", nil)
	}
	assert.Less(t, time.Since(start), time.Second,
		"Capture must drop, not block, when the queue is saturated")
	c.Close(50 * time.Millisecond)
}
