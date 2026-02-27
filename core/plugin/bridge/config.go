package bridge

import (
	"strings"
	"time"
)

// Default timeout values for the Java bridge.
const (
	DefaultStartupTimeout = 30 * time.Second
	DefaultCommandTimeout = 60 * time.Second
)

// BridgeConfig configures the bridge subprocess.
type BridgeConfig struct {
	// Command is the executable to run (e.g., "java").
	Command string

	// Args are the arguments passed to the command (e.g., ["-jar", "bridge.jar"]).
	Args []string

	// Env contains additional environment variables for the subprocess.
	// If non-empty, the subprocess inherits os.Environ() plus these entries.
	Env map[string]string

	// FilterClass is the fully-qualified Okapi filter class name.
	FilterClass string

	// StartupTimeout is how long to wait for the subprocess to become ready.
	StartupTimeout time.Duration

	// CommandTimeout is how long to wait for a single command response.
	CommandTimeout time.Duration
}

// PoolKey returns a stable key that uniquely identifies the bridge process
// configuration (command + args). Used by BridgePool for bucketing.
func (c BridgeConfig) PoolKey() string {
	return c.Command + "\x00" + strings.Join(c.Args, "\x00")
}

// streamTimeout returns the timeout for streaming RPCs (Read, Write).
// Streaming operations can transfer hundreds of thousands of messages, so they
// need a much longer deadline than unary RPCs like Open or Info.
func (c BridgeConfig) streamTimeout() time.Duration {
	return 10 * c.CommandTimeout
}

// withDefaults returns a copy of the config with zero values replaced by defaults.
func (c BridgeConfig) withDefaults() BridgeConfig {
	if c.Command == "" {
		c.Command = "java"
	}
	if c.StartupTimeout == 0 {
		c.StartupTimeout = DefaultStartupTimeout
	}
	if c.CommandTimeout == 0 {
		c.CommandTimeout = DefaultCommandTimeout
	}
	return c
}
