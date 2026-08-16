package commands

import (
	"encoding/json"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/host/venue/project"
	bconn "github.com/neokapi/neokapi/host/venue/source"
	"github.com/spf13/cobra"
)

// serverLsCmd is the plumbing behind the built-in `kapi ls` sync column: it
// emits per-file sync standing (blocks changed locally vs the last sync) as
// JSON on stdout. The built-in ls shells out to this
// (`kapi-bowrain command server-ls --json [paths...]`) when the recipe has a
// server: block and merges the dirty counts into its file listing — one
// listing across the content and transport layers, following the same
// pattern as server-status under `kapi status`.
var serverLsCmd = &cobra.Command{
	Use:    "server-ls [paths...]",
	Short:  "Emit per-file sync standing as JSON (plumbing for kapi ls)",
	Hidden: true,
	RunE:   runServerLs,
}

// serverLsJSON is the exact shape the built-in `kapi ls` merges.
type serverLsJSON struct {
	Files []serverLsFile `json:"files"`
}

type serverLsFile struct {
	Path  string `json:"path"`
	Dirty int    `json:"dirty"`
}

func runServerLs(cmd *cobra.Command, args []string) error {
	proj, err := project.FindProject("")
	if err != nil {
		return err
	}
	conn, err := bconn.NewSourceConnector(app, proj, app.FormatReg)
	if err != nil {
		// ListFiles is a local scan against the sync cache — no server
		// round-trip — so an unauthenticated (or offline) install still
		// reports sync standing. Same fallback the Mode-C daemon uses.
		conn = bconn.NewLocalConnector(app, proj, app.FormatReg)
	}
	defer conn.Close()

	files, err := conn.ListFiles(cmd.Context(), args)
	if err != nil {
		return err
	}
	out := serverLsJSON{Files: make([]serverLsFile, 0, len(files))}
	for _, f := range files {
		out.Files = append(out.Files, serverLsFile{Path: f.Path, Dirty: f.DirtyCount})
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
}

func init() {
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(serverLsCmd) })
}
