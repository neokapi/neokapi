//go:build js

package host

// execTrustImplied grants execution trust to every recipe in a browser build.
// The recipe there is written by the page itself: the lab's flow builder or a
// documentation example. A package's recipe has its exec-class steps stripped
// on ingest. No terminal is attached to ask, the process cannot start a
// subprocess, and a script step reads only the in-memory files the page gave
// it.
const execTrustImplied = true
