package output

import (
	"fmt"
	"io"
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
	// Servers are the servers the sync state among Files was issued by.
	Servers []string `json:"servers,omitempty"`
	// Taken is set when the files were taken.
	Taken bool `json:"taken"`
	// SourceRemoved is set when the source files were removed after.
	SourceRemoved bool `json:"source_removed"`
	// Reason says why nothing was taken.
	Reason string `json:"reason,omitempty"`
}

func (o StateTransferOutput) FormatText(w io.Writer) error {
	if !o.Taken {
		fmt.Fprintf(w, "Nothing taken: %s\n", o.Reason)
		return nil
	}
	fmt.Fprintf(w, "Took %d file(s) from %s into %s.\n", len(o.Files), o.From, o.To)
	if o.Action == "import" && !o.SourceRemoved {
		fmt.Fprintf(w, "The copies in %s are left as they were; kapi no longer reads them, and you can delete them.\n", o.From)
	}
	return nil
}

// StatePathOutput is the result of `kapi state path`: the directory that holds
// the checkout's own state.
type StatePathOutput struct {
	Project string `json:"project"`
	Path    string `json:"path"`
}

func (o StatePathOutput) FormatText(w io.Writer) error {
	fmt.Fprintln(w, o.Path)
	return nil
}
