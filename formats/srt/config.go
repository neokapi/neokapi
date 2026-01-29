package srt

// Config holds configuration for the SRT subtitle format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "srt" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }
