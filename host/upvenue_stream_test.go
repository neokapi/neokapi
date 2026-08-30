package host

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A run at the server venue produces the same live progress a local run does.
//
// The plumbing writes one typed event per line as the run makes progress, so
// the dispatch reads its stdout incrementally and hands each event to the
// caller's OnEvent. Read after the subprocess exits, the same lines are a
// transcript: a run view would hold a spinner for the length of a convergence
// run and then draw every event at once.

// newStreamingServerUpStub is a `server-up` stub answering with a realistic
// NDJSON document: the framing records the push phase writes, the run's
// progress events, and the closing result.
func newStreamingServerUpStub(t *testing.T) *serverUpStub {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script plugin stub is not portable to Windows")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "argv")
	binPath := filepath.Join(dir, "kapi-venue-stub")
	lines := []string{
		`{"type":"changeset","concepts_proposed":2,"changeset_id":"EMezX9AS"}`,
		`{"type":"voice_profile","voice_profile":"house","voice_profile_action":"pushed"}`,
		`{"type":"pass_start","pass":1,"maxPasses":5,"pending":["nb"]}`,
		`{"type":"locale_start","locale":"nb","units":12,"pass":1}`,
		`{"type":"unit_progress","locale":"nb","done":6,"viaTM":4,"viaAI":2,"pass":1}`,
		`{"type":"locale_done","locale":"nb","done":12,"viaTM":8,"viaAI":4,"pass":1}`,
		`{"type":"pass_done","pass":1,"produced":12,"producedDelta":12}`,
		`{"type":"materialized","files":3}`,
		`{"type":"done","state":"converged"}`,
		`{"type":"result","flow":"server-venue","passes":1,"converged":true,"materializedFiles":3}`,
	}
	var script strings.Builder
	script.WriteString("#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n")
	for _, l := range lines {
		script.WriteString("echo '" + l + "'\n")
	}
	require.NoError(t, os.WriteFile(binPath, []byte(script.String()), 0o755))

	m := &manifest.Manifest{
		ManifestVersion: manifest.CurrentVersion,
		Plugin:          "venue-stub",
		Version:         "0.0.1",
		Binary:          filepath.Base(binPath),
		Capabilities: manifest.Capabilities{
			Commands: []manifest.Command{{Name: serverUpCommand, Hidden: true}},
		},
	}
	require.NoError(t, m.Validate())

	p := &pluginhost.Plugin{
		Dir:        dir,
		BinaryPath: binPath,
		Manifest:   m,
		Source:     pluginhost.Source{Order: 1, Label: "test", Path: dir},
	}
	return &serverUpStub{host: pluginhost.NewHost([]*pluginhost.Plugin{p}, nil), argsFile: argsFile}
}

func TestRunUpDispatch_ServerVenueStreamsProgressEvents(t *testing.T) {
	recipe := newVenueTestProject(t, true)
	stub := newStreamingServerUpStub(t)
	a := &App{PluginHost: stub.host}

	var seen []convergence.Event
	out, err := a.RunUpDispatch(t.Context(), recipe, "", UpOptions{
		UntilGate: true,
		OnEvent:   func(ev convergence.Event) { seen = append(seen, ev) },
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	// The result still arrives, folded with the change-set the push phase
	// reported on its own line.
	assert.Equal(t, "server-venue", out.Flow)
	assert.True(t, out.Converged)
	assert.Equal(t, 2, out.ConceptsProposed)
	assert.Equal(t, "EMezX9AS", out.ChangesetID)

	// Every progress event reached the caller, in the order the run emitted them.
	var types []convergence.EventType
	for _, ev := range seen {
		types = append(types, ev.Type)
	}
	assert.Equal(t, []convergence.EventType{
		convergence.EventPassStart,
		convergence.EventLocaleStart,
		convergence.EventUnitProgress,
		convergence.EventLocaleDone,
		convergence.EventPassDone,
		convergence.EventMaterialized,
		convergence.EventDone,
	}, types)

	// The events are decoded, not merely counted: a surface renders their fields.
	require.Len(t, seen, 7)
	assert.Equal(t, "nb", seen[1].Locale)
	assert.Equal(t, 12, seen[1].Units)
	assert.Equal(t, 8, seen[3].ViaMemory)
	assert.Equal(t, 4, seen[3].ViaAI)
	assert.Equal(t, 3, seen[5].Files)
	assert.Equal(t, convergence.RunConverged, seen[6].State)
}

// The framing records share the document's `type` key with the run's events. A
// reader that forwarded every unrecognised line would deliver a change-set
// record to a run view as an event with every field zero.
func TestRunUpDispatch_ServerVenueForwardsOnlyRunEvents(t *testing.T) {
	recipe := newVenueTestProject(t, true)
	stub := newStreamingServerUpStub(t)
	a := &App{PluginHost: stub.host}

	var seen []convergence.Event
	_, err := a.RunUpDispatch(t.Context(), recipe, "", UpOptions{
		OnEvent: func(ev convergence.Event) { seen = append(seen, ev) },
	})
	require.NoError(t, err)
	require.NotEmpty(t, seen)
	for _, ev := range seen {
		assert.True(t, convergence.KnownEventType(ev.Type),
			"only a run's own events reach OnEvent, got %q", ev.Type)
	}
}

// OnEvent is optional, and the dispatch reads the same document either way.
func TestRunUpDispatch_ServerVenueWithoutAnEventSink(t *testing.T) {
	recipe := newVenueTestProject(t, true)
	stub := newStreamingServerUpStub(t)
	a := &App{PluginHost: stub.host}

	out, err := a.RunUpDispatch(t.Context(), recipe, "", UpOptions{})
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "server-venue", out.Flow)
	assert.Contains(t, stub.dispatchedArgs(t), "--json")
}
