package host

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/host/output"
)

// ResolveOutput is what kapi resolve did to a version of a KPZ's document
// that did not land.
type ResolveOutput struct {
	Doc  string `json:"doc"`
	Edit string `json:"edit"`
	// Action is "rebase" or "discard".
	Action string `json:"action"`
	// Carried counts the changes a rebase applied to the document, and
	// Contested names the blocks it left for a person, each with its
	// edition in parentheses when it is not the document's own.
	Carried   int      `json:"carried"`
	Contested []string `json:"contested"`
	// Refused says why the change service refused a rebase's changes;
	// nothing was settled then.
	Refused string `json:"refused,omitempty"`
	// Next is the command line that settles what is left, when anything is.
	Next string `json:"next,omitempty"`
}

// FormatText renders the outcome in a line or two.
func (o ResolveOutput) FormatText(w io.Writer) error {
	switch {
	case o.Refused != "":
		fmt.Fprintf(w, "The rebase of %s was refused, and nothing was settled: %s\n", o.Doc, o.Refused)
	case o.Action == "discard":
		fmt.Fprintf(w, "Discarded the version of %s that did not land; the document stands as it was.\n", o.Doc)
	case len(o.Contested) == 0:
		fmt.Fprintf(w, "Rebased the version of %s that did not land: %d change(s) carried over, nothing left to decide.\n", o.Doc, o.Carried)
	default:
		fmt.Fprintf(w, "Rebased the version of %s that did not land: %d change(s) carried over; %s changed in both, left to decide.\n",
			o.Doc, o.Carried, strings.Join(o.Contested, ", "))
		fmt.Fprintf(w, "Next: %s\n", o.Next)
	}
	return nil
}

// RunResolve rebases (rebase set) or discards (discard set) the write that
// did not land on doc, a document of a KPZ, as a person: kapi resolve. Inside
// a project doc is project-relative, as kapi status prints it; outside one a
// relative KPZ path is read from the working directory.
func (a *App) RunResolve(cmd Command, doc, rebase, discard string) error {
	switch {
	case rebase != "" && discard != "":
		return WithExitCode(ExitUsage, errors.New("name one of --rebase and --discard"))
	case rebase == "" && discard == "":
		return WithExitCode(ExitUsage, errors.New("name the write to settle with --rebase WRITE or --discard WRITE, as kapi status prints it"))
	}
	ctx := CmdContext(cmd)
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		recipe = ""
	}
	if recipe == "" {
		if doc, err = absoluteKpzRef(doc); err != nil {
			return WithExitCode(ExitUsage, err)
		}
	}
	out := ResolveOutput{Doc: doc, Edit: rebase + discard, Contested: []string{}}
	if discard != "" {
		out.Action = "discard"
		if err := a.DiscardKeptDocument(ctx, recipe, doc, discard, "resolve"); err != nil {
			return resolveError(err)
		}
		return output.Print(cmd, out)
	}
	out.Action = "rebase"
	d, key, _, err := a.projectKpzDocuments(ctx, recipe, doc)
	if err != nil {
		return resolveError(err)
	}
	rb, err := a.RebaseKeptDocument(ctx, recipe, doc, rebase, "resolve")
	if err != nil {
		return resolveError(err)
	}
	out.Carried, out.Refused = rb.Carried, rb.Refused
	if rb.Contested > 0 {
		head, _, _, err := d.Head(ctx, key)
		if err != nil {
			return err
		}
		for _, dv := range head.Divergent {
			if dv.Op == rebase {
				sc := documentStatusConflict(doc, dv)
				out.Contested, out.Next = sc.Blocks, sc.Next
			}
		}
	}
	if err := output.Print(cmd, out); err != nil {
		return err
	}
	if out.Refused != "" {
		return WithExitCode(ExitGate, errors.New("the rebase was refused"))
	}
	return nil
}

// resolveError gives a reference or a write kapi resolve cannot settle the
// usage exit status.
func resolveError(err error) error {
	if errors.Is(err, ErrNotKpzDocument) || errors.Is(err, workhome.ErrNotDivergent) {
		return WithExitCode(ExitUsage, err)
	}
	return err
}

// absoluteKpzRef names the KPZ in doc by its absolute path, read from the
// working directory.
func absoluteKpzRef(doc string) (string, error) {
	i := strings.Index(strings.ToLower(doc), workspaceExt+"!")
	if i < 0 {
		return "", fmt.Errorf("%s names no document of a .kpz (work.kpz!guide.md)", doc)
	}
	kpzPath := doc[:i+len(workspaceExt)]
	if !filepath.IsAbs(kpzPath) {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		kpzPath = filepath.Join(wd, kpzPath)
	}
	return kpzPath + doc[i+len(workspaceExt):], nil
}
