package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The desktop backend emits events through App.emit, which fans out to the
// Wails runtime (when running as the app) and to an optional event sink (used
// by the recording wbridge, which has no Wails runtime). These verify emit is
// safe with neither wired, that a registered sink receives name+data, and that
// clearing the sink stops delivery.
func TestEmit_NoAppNoSink_DoesNotPanic(t *testing.T) {
	a := &App{}
	assert.NotPanics(t, func() {
		a.emit("connection-state-changed", ConnectionInfo{State: StateConnected})
	})
}

func TestSetEventSink_ReceivesEvents(t *testing.T) {
	a := &App{}

	type rec struct {
		name string
		data any
	}
	var got []rec
	a.SetEventSink(func(name string, data any) {
		got = append(got, rec{name, data})
	})

	a.emit("connection-state-changed", ConnectionInfo{State: StateConnected, ServerURL: "http://localhost:8080"})
	a.emit("blocks-changed", BlockChangedEvent{ItemName: "about-us.html"})

	require.Len(t, got, 2)

	assert.Equal(t, "connection-state-changed", got[0].name)
	ci, ok := got[0].data.(ConnectionInfo)
	require.True(t, ok, "first event should carry a ConnectionInfo")
	assert.Equal(t, StateConnected, ci.State)
	assert.Equal(t, "http://localhost:8080", ci.ServerURL)

	assert.Equal(t, "blocks-changed", got[1].name)
	bc, ok := got[1].data.(BlockChangedEvent)
	require.True(t, ok, "second event should carry a BlockChangedEvent")
	assert.Equal(t, "about-us.html", bc.ItemName)
}

func TestSetEventSink_NilClearsDelivery(t *testing.T) {
	a := &App{}
	var count int
	a.SetEventSink(func(string, any) { count++ })
	a.emit("connection-state-changed", ConnectionInfo{})
	require.Equal(t, 1, count)

	a.SetEventSink(nil)
	a.emit("connection-state-changed", ConnectionInfo{})
	assert.Equal(t, 1, count, "no further events after the sink is cleared")
}
