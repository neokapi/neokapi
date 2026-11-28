package output

import (
	"fmt"
	"io"
	"strings"
)

// StateFileOutput is one file `kapi state import` or `kapi state relink`
// takes: where it is, where it goes in the checkout's state directory, and its
// size in bytes.
type StateFileOutput struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Size   int64  `json:"size"`
}

// StateTransferOutput is the result of `kapi state import` and
// `kapi state relink`.
type StateTransferOutput struct {
	// Action is "import" or "relink".
	Action string `json:"action"`
	// From is the directory the files come from.
	From string `json:"from"`
	// To is the checkout's state directory.
	To    string            `json:"to"`
	Files []StateFileOutput `json:"files"`
	// Servers are the servers the sync state among Files and Withheld was
	// issued by.
	Servers []string `json:"servers,omitempty"`
	// Project names the project a relink's state was recorded for.
	Project string `json:"project,omitempty"`
	// Unverified is set when kapi's record cannot say the state is this
	// project's.
	Unverified bool `json:"unverified,omitempty"`
	// Withheld are the files a relink leaves where they are, and WithheldWhy
	// says why, one line for each group.
	Withheld    []StateFileOutput `json:"withheld,omitempty"`
	WithheldWhy []string          `json:"withheld_why,omitempty"`
	// SyncServer and SyncProject name the server project the sync state among
	// the files is bound to, when its sync cache names one. SyncProject is
	// <workspace>/<project> when the cache records the workspace.
	SyncServer  string `json:"sync_server,omitempty"`
	SyncProject string `json:"sync_project,omitempty"`
	// RemovesSource is set when a relink removes the directory it takes from,
	// once every file is in place and read back.
	RemovesSource bool `json:"removes_source,omitempty"`
	// Taken is set when the files were taken.
	Taken bool `json:"taken"`
	// SourceRemoved is set when the source files were removed after.
	SourceRemoved bool `json:"source_removed"`
	// Remaining are the copies an import was asked to remove and could not.
	Remaining []string `json:"remaining,omitempty"`
	// Reason says why nothing was taken.
	Reason string `json:"reason,omitempty"`
}

func (o StateTransferOutput) FormatText(w io.Writer) error {
	if !o.Taken {
		fmt.Fprintf(w, "Nothing taken: %s\n", o.Reason)
		return nil
	}
	fmt.Fprintf(w, "Took %d file(s) from %s into %s.\n", len(o.Files), o.From, o.To)
	switch {
	case len(o.Remaining) > 0:
		fmt.Fprintf(w, "These copies in %s could not be removed; kapi no longer reads them, and you can delete them: %s\n", o.From, strings.Join(o.Remaining, ", "))
	case o.Action == "import" && !o.SourceRemoved:
		fmt.Fprintf(w, "The copies in %s are left as they were; kapi no longer reads them, and you can delete them.\n", o.From)
	}
	if len(o.Withheld) > 0 {
		fmt.Fprintf(w, "%d file(s) stay in %s:\n", len(o.Withheld), o.From)
		for _, why := range o.WithheldWhy {
			fmt.Fprintf(w, "  %s\n", why)
		}
	}
	return nil
}

// StatePathOutput is the result of `kapi state path`: the directory that holds
// the checkout's own state.
type StatePathOutput struct {
	Project string `json:"project"`
	Path    string `json:"path"`
	// ParseCache is the directory the parse cache lives in: work/cache/docs
	// under Path, or the directory $KAPI_PARSE_CACHE_DIR names.
	ParseCache string `json:"parse_cache"`
}

func (o StatePathOutput) FormatText(w io.Writer) error {
	fmt.Fprintln(w, o.Path)
	return nil
}
