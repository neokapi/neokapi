package host

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// Every message about content no installed reader opens names the plugin to
// install through one resolution, and an installed plugin that declares a
// format is the plugin named for it.

func TestInstalledPluginIsNamedForTheFormatItDeclares(t *testing.T) {
	acme := []*pluginhost.Plugin{{Manifest: &manifest.Manifest{
		Plugin:       "acme",
		Capabilities: manifest.Capabilities{Formats: []manifest.Format{{Name: "acme_doc"}}},
	}}}

	assert.Contains(t, NoReaderError(errNoReader, "doc", "acme_doc", acme...).Error(), "(kapi plugins install acme)")
	assert.Contains(t, noReaderReason(errNoReader, "acme_doc", acme...), "(kapi plugins install acme)")

	u := NewUnreadSet(acme...)
	require.True(t, u.Skip(errNoReader, "x.acme", "acme_doc"))
	require.Len(t, u.warnings(), 1)
	assert.Contains(t, u.warnings()[0].Message, "(kapi plugins install acme)")

	a := &App{PluginHost: pluginhost.NewHost(acme, func(string) {})}
	fromApp := a.newUnreadSet()
	require.True(t, fromApp.Skip(errNoReader, "x.acme", "acme_doc"))
	assert.Contains(t, fromApp.warnings()[0].Message, "(kapi plugins install acme)", "a set the App builds reads its discovered plugins")
}

func TestEveryNoReaderMessageNamesThePlugin(t *testing.T) {
	a := &App{}

	var status bytes.Buffer
	statusCmd := NewEnvCommand(context.Background(), "status")
	statusCmd.SetErr(&status)
	a.warnUnreadableFormats(statusCmd, []string{"okf_idml", "frob"})
	assert.Contains(t, status.String(), `no reader for format "okf_idml", so content declaring it was not measured; install the plugin that supplies it (kapi plugins install okapi-bridge)`)
	assert.Contains(t, status.String(), `no reader for format "frob", so content declaring it was not measured; no known plugin supplies it`)

	u := NewUnreadSet()
	require.True(t, u.Skip(errNoReader, "pkg/doc.idml", "okf_idml"))
	var warned bytes.Buffer
	checkCmd := NewEnvCommand(context.Background(), "check")
	checkCmd.SetErr(&warned)
	u.warn(a, checkCmd)
	assert.Contains(t, warned.String(), `no reader for format "okf_idml", so pkg/doc.idml was not checked; install the plugin that supplies it (kapi plugins install okapi-bridge)`)

	var events []convergence.Event
	announceSetAside(convergence.NewEmitter(func(ev convergence.Event) { events = append(events, ev) }), u)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].Message, "Install the plugin that supplies it (kapi plugins install okapi-bridge).")

	assert.Equal(t, `No reader for format "okf_idml": its source is not settled or counted before translation. Install the plugin that supplies it (kapi plugins install okapi-bridge).`,
		translateAfterUnreadMessage(nil, "okf_idml"))
	assert.Equal(t, `No reader for format "frob": its source is not settled or counted before translation. No known plugin supplies it.`,
		translateAfterUnreadMessage(nil, "frob"))
}
