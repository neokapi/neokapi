package host

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// MergeServerLs folds per-file sync standing into the ls listing when the
// recipe binds a convergence venue and a plugin provides the hidden server-ls
// plumbing. It shells `server-ls --json --project=<recipe> [paths...]`
// (subprocess dispatch — the cli module never imports bowrain), handing the
// plugin the recipe ls resolved and the files ls listed (serverLsArgs), and
// sets each entry's Dirty count.
// Any failure degrades to a one-line stderr warning and leaves the local
// listing intact: ls must never fail on a server hiccup. Mirrors
// appendServerStatus under `kapi status`.
func (a *App) MergeServerLs(cmd Command, recipePath string, proj *project.KapiProject, out *output.LsOutput, paths []string) {
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
	raw, err := route.CaptureStdout(cmd.Context(), append([]string{"--json", "--project=" + recipePath}, serverLsArgs(out, paths)...)...)
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

// serverLsArgMax bounds the bytes of file paths handed to server-ls on its
// command line, under the smallest limit a platform puts on one (Windows takes
// 32,767 characters for the whole line).
var serverLsArgMax = func() int {
	if runtime.GOOS == "windows" {
		return 24 << 10
	}
	return 256 << 10
}()

// serverLsArgs is what server-ls is handed to list: the files ls already
// resolved, so the plugin reads exactly those rather than matching every
// pattern again. A list too long for a command line hands over the user's
// path arguments instead, and the plugin resolves them itself.
func serverLsArgs(out *output.LsOutput, paths []string) []string {
	if len(out.Files) == 0 {
		return paths
	}
	files := make([]string, 0, len(out.Files))
	size := 0
	for _, f := range out.Files {
		size += len(f.Path) + 1
		if size > serverLsArgMax {
			return paths
		}
		files = append(files, f.Path)
	}
	return files
}
