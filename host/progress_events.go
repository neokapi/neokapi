package host

import (
	"encoding/json"
	"sync"
)

// progressFlagName is the NDJSON progress flag shared by run/extract/merge.
const progressFlagName = "progress"

// addProgressFlag registers --progress on a long-running verb. The only
// mode is "jsonl": stream machine-readable progress events to stderr, one
// JSON object per line, using the FlowRunEvent vocabulary (cli/flowrun.go) —
// the same event shapes the desktop run sink receives.
func AddProgressFlag(cmd Command) {
	cmd.Flags().String(progressFlagName, "",
		"stream progress events to stderr as NDJSON (value: jsonl); events use the flow-run event vocabulary")
}

// progressSink resolves the --progress flag into a RunEventSink. It returns
// nil (the discard sink) unless --progress=jsonl was passed. Events go to
// stderr so stdout stays reserved for the command's result (text or --json).
// The sink is safe for concurrent emitters.
func ProgressSink(cmd Command) RunEventSink {
	mode, _ := cmd.Flags().GetString(progressFlagName)
	if mode != "jsonl" {
		return nil
	}
	enc := json.NewEncoder(cmd.ErrOrStderr())
	var mu sync.Mutex
	return func(ev FlowRunEvent) {
		mu.Lock()
		defer mu.Unlock()
		_ = enc.Encode(ev)
	}
}
