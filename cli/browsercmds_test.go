package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSurfaceApp builds the App the two command-set constructors expect: the
// registries populated (NewToolCommands enumerates the tool registry) but no
// plugin host, config, or credentials — construction must not need them.
func newSurfaceApp(t *testing.T) *App {
	t.Helper()
	a := &App{}
	a.InitRegistries()
	return a
}

func commandsByName(cmds []*cobra.Command) map[string]*cobra.Command {
	byName := make(map[string]*cobra.Command, len(cmds))
	for _, c := range cmds {
		byName[c.Name()] = c
	}
	return byName
}

// TestBrowserCommandSurface is the drift guard between the native kapi command
// surface (KapiCommandSet, what kapi/cmd/kapi registers) and the browser one
// (BrowserCommandSet, what kapi/cmd/kapi-wasm-cli registers).
//
// The browser build used to hand-roll its own command list, so a verb renamed
// or added natively silently vanished from the lab — `kapi convert` became
// `kapi kconv` in #1180 and the Conversion Lab reported `unknown command
// "convert" for "kapi"` for weeks. This test makes that impossible: every
// native verb must be present in the browser set, either as a working command
// or as a recorded gap that explains itself.
func TestBrowserCommandSurface(t *testing.T) {
	native := commandsByName(KapiCommandSet(newSurfaceApp(t)))
	browser := commandsByName(BrowserCommandSet(newSurfaceApp(t)))

	require.NotEmpty(t, native, "the native command set must not be empty")

	t.Run("every native verb exists in the browser build", func(t *testing.T) {
		for name := range native {
			assert.Contains(t, browser, name,
				"%[1]q is registered natively but not in the browser build — a lab user "+
					"typing it gets cobra's `unknown command`. Either wire %[1]q into "+
					"BrowserCommandSet, or record it in browserGaps with the reason it "+
					"cannot run in the browser.", name)
		}
	})

	t.Run("the browser build registers no verb the native build lacks", func(t *testing.T) {
		for name := range browser {
			assert.Contains(t, native, name,
				"%q is registered in the browser build but not natively — it is either a "+
					"retired verb the browser build kept, or a new verb that never reached "+
					"KapiCommandSet.", name)
		}
	})

	t.Run("recorded gaps mirror their native command's help metadata", func(t *testing.T) {
		for name, gap := range browserGaps {
			if !assert.Contains(t, native, name,
				"browserGaps records %q, which no longer exists natively — drop the entry", name) {
				continue
			}
			nativeCmd := native[name]
			assert.Equal(t, nativeCmd.Short, gap.short,
				"browserGaps[%q].short drifted from the native command's Short; "+
					"`kapi --help` would read differently in the lab", name)
			assert.Equal(t, nativeCmd.GroupID, gap.group,
				"browserGaps[%q].group drifted from the native command's GroupID", name)
			assert.Equal(t, nativeCmd.Hidden, gap.hidden,
				"browserGaps[%q].hidden drifted from the native command's Hidden", name)
			assert.NotEmpty(t, gap.reason, "browserGaps[%q] must explain why the verb cannot run", name)
		}
	})

	t.Run("gap commands fail with an honest, verb-specific message", func(t *testing.T) {
		for name, gap := range browserGaps {
			cmd, ok := browser[name]
			require.True(t, ok, "%q must be registered in the browser set as a gap command", name)

			// Drive the command the way the lab terminal does, including the
			// subcommand and flag spellings a user would reach for. None of them
			// may fall through to "unknown command".
			for _, argv := range [][]string{nil, {"list"}, {"list", "--json"}, {"--wat"}} {
				root := &cobra.Command{Use: "kapi", SilenceUsage: true, SilenceErrors: true}
				AddCommandGroups(nil, root)
				root.AddCommand(cmd)
				root.SetArgs(append([]string{name}, argv...))
				root.SetOut(&bytes.Buffer{})
				root.SetErr(&bytes.Buffer{})

				err := root.Execute()
				require.Error(t, err, "`kapi %s %v` must fail explicitly, not succeed silently", name, argv)
				assert.NotContains(t, err.Error(), "unknown command",
					"`kapi %s %v` must explain the browser limitation, not deny the verb exists", name, argv)
				assert.Equal(t, BrowserUnavailableError(name, gap.reason).Error(), err.Error(),
					"`kapi %s %v` must return the canonical browser-unavailable error", name, argv)
			}
		}
	})

	t.Run("gap commands still answer --help, and the help names the limitation", func(t *testing.T) {
		for name, gap := range browserGaps {
			cmd, ok := browser[name]
			require.True(t, ok, "%q must be registered in the browser set as a gap command", name)

			for _, flag := range []string{"--help", "-h"} {
				root := &cobra.Command{Use: "kapi", SilenceUsage: true, SilenceErrors: true}
				AddCommandGroups(nil, root)
				root.AddCommand(cmd)
				var stdout bytes.Buffer
				root.SetArgs([]string{name, flag})
				root.SetOut(&stdout)
				root.SetErr(&stdout)

				require.NoError(t, root.Execute(),
					"`kapi %s %s` must print help — a reader asking what a verb does deserves an answer", name, flag)
				assert.Contains(t, stdout.String(), gap.reason,
					"`kapi %s %s` must explain the browser limitation in its help", name, flag)
			}
		}
	})

	t.Run("browser-supported verbs are real commands, not gaps", func(t *testing.T) {
		for name, cmd := range browser {
			if _, isGap := browserGaps[name]; isGap {
				continue
			}
			// A real command either runs something or hosts subcommands. A verb
			// that does neither would fall back to cobra's help with exit 0 — the
			// silent-success failure mode this test exists to prevent.
			assert.True(t, cmd.RunE != nil || cmd.Run != nil || cmd.HasSubCommands(),
				"browser command %q does nothing: no RunE, no Run, no subcommands", name)
		}
	})
}

// TestBrowserCommandSetOrder pins that the browser set is registered in the
// same order as the native one for the verbs they share, so `kapi --help` lists
// commands identically in the lab and on the desktop.
func TestBrowserCommandSetOrder(t *testing.T) {
	var nativeOrder, browserOrder []string
	for _, c := range KapiCommandSet(newSurfaceApp(t)) {
		nativeOrder = append(nativeOrder, c.Name())
	}
	for _, c := range BrowserCommandSet(newSurfaceApp(t)) {
		browserOrder = append(browserOrder, c.Name())
	}
	assert.Equal(t, nativeOrder, browserOrder,
		"BrowserCommandSet must mirror KapiCommandSet's registration order so the two "+
			"functions stay readable side by side and --help renders the same")
}

// TestBrowserGapCmdPanicsOnUnrecordedVerb documents the constructor's contract:
// a gap command may only be built for a verb the table explains.
func TestBrowserGapCmdPanicsOnUnrecordedVerb(t *testing.T) {
	assert.PanicsWithValue(t, "cli: no browser gap recorded for command nope", func() {
		newBrowserGapCmd("nope")
	})
}
