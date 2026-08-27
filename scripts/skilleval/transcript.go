package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Recording the session, rather than a summary of it.
//
// The agent runs with --output-format stream-json, so the whole conversation
// goes past: every assistant message, every tool call with its arguments, and
// every tool result, which is what the agent actually saw before its next move.
// Run keeps counts and a deduplicated tool list out of that, which answers "what
// did it reach for" and cannot answer "why did it do that".
//
// The second question is the one a reader has when a verdict surprises them, and
// it is the question the eval work kept needing: #2227 was diagnosed from a
// transcript showing an agent produce a correct change-set and then spend a
// 40-turn budget on one rejection. That reading happened on a developer's
// machine, from a run nobody else could see.
//
// So the events are kept, and they are kept out of the dataset: the page imports
// _skilleval.json directly, and sessions are large. Each scenario's sessions are
// written to their own file under web/static, and the row fetches it when a
// reader opens it. The index stays the size it is; the evidence is one click
// away rather than one machine away.

// The session is published whole.
//
// It was capped — 400 events, 256KB, each message clipped to 3,000 characters
// and each tool result to 1,200 — because it was committed to git. Those caps
// cut exactly the part a reader wants: the file the agent actually read, the
// error it actually got. A transcript that stops mid-tool-result is a summary
// with a scrollbar.
//
// So the transcripts go where the artefacts go, an S3 bucket behind CloudFront,
// and nothing is truncated on the way. What remains is scrubbing, which is not
// a cap: a published transcript must not carry the machine it ran on.

// Event is one step of a session, in the order it happened.
type Event struct {
	// Kind is "text" for an assistant message or "tool" for a call and the
	// result it returned.
	Kind string `json:"kind"`
	// Name is the tool, on a tool event.
	Name string `json:"name,omitempty"`
	// Text is the assistant's message.
	Text string `json:"text,omitempty"`
	// Input is the call's arguments, as the JSON the agent sent.
	Input string `json:"input,omitempty"`
	// Output is what came back, which is what the agent read before its next
	// step. It arrives in a later event than the call, and is matched to it by
	// the id the stream carries on both.
	Output string `json:"output,omitempty"`
	// Failed marks a result the harness returned as an error.
	Failed bool `json:"failed,omitempty"`
}

// record appends an event, scrubbed.
//
// Every string reaching an Event goes through here, so no caller has to
// remember to scrub: a transcript carries the workspace path, the developer's
// home, and whatever the agent printed of both.
func (r *Run) record(e Event) {
	e.Text = scrubPaths(e.Text)
	e.Input = scrubPaths(e.Input)
	e.Output = scrubPaths(e.Output)
	r.Events = append(r.Events, e)
}

// recordResult fills in the output of an earlier call.
func (r *Run) recordResult(idx int, out string, failed bool) {
	if idx < 0 || idx >= len(r.Events) {
		return
	}
	e := &r.Events[idx]
	e.Output = scrubPaths(out)
	e.Failed = failed
}

// Session is one run's recorded events, whole.
type Session struct {
	Events []Event `json:"events"`
}

// SessionFile is one scenario's sessions, both arms, as published.
type SessionFile struct {
	Key      string `json:"key"`
	Scenario string `json:"scenario"`
	// Prompt is the turn the conversation opens on, so the page can render it
	// as the user message it was.
	Prompt  string    `json:"prompt"`
	Runs    []Session `json:"runs"`
	Unaided []Session `json:"unaided,omitempty"`
}

// splitSessions moves every run's events out of the report and into one file
// per scenario, leaving each result pointing at its file.
//
// The dataset is imported by the page rather than fetched, so it is in the JS
// bundle: an inline session would put a hundred file reads in front of a reader
// who wanted four numbers.
func splitSessions(r *Report) map[string]*SessionFile {
	out := map[string]*SessionFile{}
	for i := range r.Results {
		res := &r.Results[i]
		f := &SessionFile{Key: r.Key(), Scenario: res.Scenario.ID, Prompt: res.Scenario.Prompt}
		f.Runs = takeSessions(res.Runs)
		f.Unaided = takeSessions(res.Unaided)
		if !hasEvents(f) {
			continue
		}
		name := sessionFileName(r.Key(), res.Scenario.ID)
		out[name] = f
		res.Transcript = name
	}
	return out
}

func takeSessions(runs []Run) []Session {
	out := make([]Session, 0, len(runs))
	for i := range runs {
		out = append(out, Session{Events: runs[i].Events})
		runs[i].Events = nil
	}
	return out
}

func hasEvents(f *SessionFile) bool {
	for _, set := range [][]Session{f.Runs, f.Unaided} {
		for _, s := range set {
			if len(s.Events) > 0 {
				return true
			}
		}
	}
	return false
}

// sessionFileName is stable across runs, so a re-run overwrites rather than
// accumulating, and safe as a path and a URL.
func sessionFileName(key, scenario string) string {
	return sessionFilePrefix(key) + slug(scenario) + ".json"
}

// sessionFilePrefix is what every file for one surface and mode starts with,
// and so what pruning matches on.
func sessionFilePrefix(key string) string { return slug(key) + "--" }

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// writeSessions writes the files and removes the ones this key no longer has.
//
// Pruning is scoped to the key being written for the same reason merge() is: a
// `-only mcp` run must not delete the skill surface's transcripts, which it did
// not measure and cannot replace.
func writeSessions(dir, key string, files map[string]*SessionFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	existing, err := filepath.Glob(filepath.Join(dir, sessionFilePrefix(key)+"*.json"))
	if err != nil {
		return err
	}
	for _, p := range existing {
		if _, keep := files[filepath.Base(p)]; !keep {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	for name, f := range files {
		body, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(body, '\n'), 0o644); err != nil {
			return fmt.Errorf("write transcript %s: %w", name, err)
		}
	}
	return nil
}
