package host

import (
	"encoding/json"
	"fmt"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// MergeServerLs folds per-file sync standing into the ls listing when the
// recipe binds a convergence venue and a plugin provides the hidden server-ls
// plumbing. It shells `server-ls --json [paths...]` (subprocess dispatch —
// the cli module never imports bowrain) and sets each entry's Dirty count.
// Any failure degrades to a one-line stderr warning and leaves the local
// listing intact: ls must never fail on a server hiccup. Mirrors
// appendServerStatus under `kapi status`.
func (a *App) MergeServerLs(cmd Command, proj *project.KapiProject, out *output.LsOutput, paths []string) {
	if _, ok := proj.Venue(); !ok {
		return
	}
	if a.PluginHost == nil {
		return
	}
	route := a.PluginHost.CommandRoute("server-ls")
	if route == nil {
		return
	}
	raw, err := route.CaptureStdout(cmd.Context(), append([]string{"--json"}, paths...)...)
	if err != nil {
		if !a.Quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not read per-file sync standing: %v\n", err)
		}
		return
	}
	var payload struct {
		Files []struct {
			Path  string `json:"path"`
			Dirty int    `json:"dirty"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		if !a.Quiet {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not parse per-file sync standing: %v\n", err)
		}
		return
	}
	dirty := make(map[string]int, len(payload.Files))
	for _, f := range payload.Files {
		dirty[f.Path] = f.Dirty
	}
	for i := range out.Files {
		if d, ok := dirty[out.Files[i].Path]; ok {
			out.Files[i].Dirty = &d
		}
	}
	out.HasSync = true
}
