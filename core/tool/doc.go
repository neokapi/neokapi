// Package tool defines the [Tool] interface for processing Parts in a Flow.
// Each Tool reads Parts from an input channel, transforms them, and writes
// results to an output channel. [BaseTool] provides default pass-through
// behavior; concrete tools embed it and set handler functions for the Part
// types they care about.
package tool
