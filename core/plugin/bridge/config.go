package bridge

import "time"

// Default timeout values for the Java bridge.
const (
	DefaultStartupTimeout = 30 * time.Second
	DefaultCommandTimeout = 60 * time.Second
)

// BridgeConfig configures the Java bridge subprocess.
type BridgeConfig struct {
	// JavaPath is the path to the java binary. Default: "java".
	JavaPath string

	// JARPath is the path to the gokapi-bridge fat JAR.
	JARPath string

	// JVMArgs are extra arguments passed to the JVM (e.g., "-Xmx512m").
	JVMArgs []string

	// FilterClass is the fully-qualified Okapi filter class name.
	FilterClass string

	// StartupTimeout is how long to wait for the JVM to become ready.
	StartupTimeout time.Duration

	// CommandTimeout is how long to wait for a single command response.
	CommandTimeout time.Duration
}

// withDefaults returns a copy of the config with zero values replaced by defaults.
func (c BridgeConfig) withDefaults() BridgeConfig {
	if c.JavaPath == "" {
		c.JavaPath = "java"
	}
	if c.StartupTimeout == 0 {
		c.StartupTimeout = DefaultStartupTimeout
	}
	if c.CommandTimeout == 0 {
		c.CommandTimeout = DefaultCommandTimeout
	}
	return c
}
