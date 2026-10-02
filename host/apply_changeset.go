package host

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changeschema"
	"github.com/neokapi/neokapi/core/contextop"
)

// ApplyOptions are the flags of one kapi apply run.
type ApplyOptions struct {
	// DryRun previews the change set: it is computed and checked, nothing is
	// written, and the result carries a diff per document.
	DryRun bool
	// Gate, when set, takes the place of the change set's own gate. A person
	// sets report to land a change whose findings would refuse it.
	Gate change.Gate
	// JSON prints the result as kapi.change-result/v1.
	JSON bool
	// PrintOps prints the decoded change set, defaults filled in, and applies
	// nothing.
	PrintOps bool
	// BackupSuffix keeps a copy of each file the change replaces, beside it
	// with the suffix appended.
	BackupSuffix string
}

// RunApplySchema prints the JSON Schema of a kapi.change/v1 change set.
func RunApplySchema(w io.Writer) error {
	_, err := w.Write(append(changeschema.Schema(), '\n'))
	return err
}

// RunApply applies a kapi.change/v1 change set read from path, or from
// standard input when path is "" or "-", through the change service. The
// command line stamps the actor: a person, or the agent session the
// environment names. It exits 0 when the change set applied or previewed, 2
// when it does not decode or contradicts itself, 3 when an operation was
// refused (nothing is written) or the change landed in part, and 5 when a
// backend did not answer.
func (a *App) RunApply(cmd Command, path string, opts ApplyOptions) error {
	ctx := cmd.Context()
	data, err := readContent(ctx, path)
	if err != nil {
		return err
	}
	if err := retiredChangeShape(data); err != nil {
		return WithExitCode(ExitUsage, err)
	}
	set, err := change.Decode(bytes.NewReader(data))
	if err != nil {
		return WithExitCode(ExitUsage, fmt.Errorf("apply: %w", err))
	}
	if opts.DryRun {
		set.Mode = change.ModePreview
	}
	if opts.Gate != "" {
		set.Gate = opts.Gate
	}
	if opts.PrintOps {
		return writeChangeSet(cmd.OutOrStdout(), set)
	}

	resolved, err := a.commandActor()
	if err != nil {
		return err
	}
	actor := changeActorOf(resolved.Actor)
	// The note each record carries says what applied the change when the
	// change set gives none, and keeps that a person was at the keyboard of
	// an agent host's shell.
	if set.Note == "" {
		set.Note = "applied with `kapi apply`"
	}
	set.Note = resolved.NoteWith(set.Note)
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	root, err := changeRoot(recipe)
	if err != nil {
		return err
	}

	comments, err := a.commentSet(set, root)
	if err != nil {
		return WithExitCode(ExitUsage, err)
	}
	var res *change.Result
	if comments {
		trust := a.applyFormatterTrust(cmd, path == "" || path == StdinName)
		res, err = a.applyCommentSet(ctx, cmd, set, root, opts.BackupSuffix, trust)
	} else {
		var svc *change.Service
		svc, err = a.changeService(ctx, cmd, ChangeServiceOptions{
			Project: recipe, Origin: "apply", Format: a.FormatFlag,
			AnyPath: recipe == "", BackupSuffix: opts.BackupSuffix,
		})
		if err != nil {
			return err
		}
		res, err = svc.Apply(ctx, set, actor)
	}
	if err != nil {
		return err
	}
	if res.Status == change.SetApplied || res.Status == change.SetPartial {
		a.noteAgentEdits(ctx, recipe, resolved.Actor, appliedWordings(set, res))
	}

	if opts.JSON {
		if err := writeChangeResult(cmd.OutOrStdout(), res); err != nil {
			return err
		}
	} else {
		if set.Mode == change.ModePreview {
			for _, d := range res.Docs {
				fmt.Fprint(cmd.OutOrStdout(), d.Diff)
			}
		}
		printChangeResult(cmd.ErrOrStderr(), res)
	}
	return changeResultExit(res)
}

// changeActorOf is the change service's actor for a context actor.
func changeActorOf(a contextop.Actor) change.Actor {
	return change.Actor{Kind: change.ActorKind(a.Kind), Name: a.Name, Session: a.Session}
}

// changeRoot is the directory a change set's document references resolve
// under: the project's root, or the working directory outside a project.
func changeRoot(recipe string) (string, error) {
	if recipe != "" {
		return filepath.Dir(recipe), nil
	}
	return os.Getwd()
}

// writeChangeSet prints a change set as one indented JSON object, the form
// kapi apply reads back.
func writeChangeSet(w io.Writer, set change.Set) error {
	raw, err := set.MarshalJSON()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return err
	}
	buf.WriteByte('\n')
	_, err = w.Write(buf.Bytes())
	return err
}

// writeChangeResult prints a result as indented JSON with markup left
// readable.
func writeChangeResult(w io.Writer, res *change.Result) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

// changeResultExit is the exit a command returns for a result: nil when the
// change set applied or previewed; the exit code of the refusal that stopped
// it (2 for a change set that is invalid, 5 for an unreachable backend, 3
// otherwise); 3 when it landed in part.
func changeResultExit(res *change.Result) error {
	switch res.Status {
	case change.SetApplied, change.SetPreviewed:
		return nil
	case change.SetPartial:
		return WithExitCode(ExitGate, ErrSilentExit)
	}
	code := change.ExitRefused
	for _, op := range res.Ops {
		if op.Status == change.OpRefused && op.Error != nil {
			code = op.Error.Code.ExitCode()
			break
		}
	}
	return WithExitCode(code, ErrSilentExit)
}

// printChangeResult writes a short account of a result for a person: one line
// per operation that did not simply apply, each document written, and the
// outcome.
func printChangeResult(w io.Writer, res *change.Result) {
	counts := map[change.OpStatus]int{}
	for _, op := range res.Ops {
		counts[op.Status]++
		at := ""
		if op.At != nil {
			at = " " + refLabel(*op.At)
		}
		switch op.Status {
		case change.OpRefused:
			msg := ""
			if op.Error != nil {
				msg = ": " + op.Error.Error()
			}
			fmt.Fprintf(w, "op %d %s%s: refused%s\n", op.I, op.Op, at, msg)
			if op.Current != nil {
				fmt.Fprintf(w, "  current %s: %s\n", op.Current.Rev, op.Current.Text)
			}
			for _, c := range opCandidates(op) {
				fmt.Fprintf(w, "  candidate %s\n", c)
			}
		case change.OpNotApplied:
			fmt.Fprintf(w, "op %d %s%s: not applied\n", op.I, op.Op, at)
		}
		for _, f := range op.Findings {
			fmt.Fprintf(w, "op %d %s%s: %s %s: %s\n", op.I, op.Op, at, findingOutcome(f), f.Rule, f.Message)
		}
	}
	for _, d := range res.Docs {
		label := d.Doc
		if d.File != "" {
			label = d.File
		}
		if d.Written {
			fmt.Fprintf(w, "wrote %s\n", label)
		}
		for _, f := range d.Findings {
			fmt.Fprintf(w, "%s: %s %s: %s\n", label, findingOutcome(f), f.Rule, f.Message)
		}
	}
	parts := []string{}
	for _, s := range []change.OpStatus{change.OpApplied, change.OpPreviewed, change.OpUnchanged, change.OpRefused, change.OpNotApplied} {
		if n := counts[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, strings.ReplaceAll(string(s), "_", " ")))
		}
	}
	fmt.Fprintf(w, "change set %s: %s\n", res.Status, strings.Join(parts, ", "))
}

// opCandidates lists what a refused reference or find could have meant.
func opCandidates(op change.OpResult) []string {
	if op.Error == nil {
		return nil
	}
	out := make([]string, 0, len(op.Error.Candidates))
	for _, c := range op.Error.Candidates {
		var b strings.Builder
		if c.Key != "" {
			b.WriteString(c.Key)
		}
		if c.Occurrence > 0 {
			fmt.Fprintf(&b, "occurrence %d", c.Occurrence)
		}
		if c.Rev != "" {
			b.WriteString(" " + c.Rev)
		}
		if c.Text != "" {
			b.WriteString(": " + c.Text)
		}
		out = append(out, strings.TrimSpace(b.String()))
	}
	return out
}

func findingOutcome(f change.Finding) string {
	switch {
	case f.Suggested:
		return "suggests"
	case f.Fails:
		return "fails"
	}
	return "reports"
}

// refLabel names a reference in one line of text.
func refLabel(r change.Ref) string {
	s := r.Doc
	if r.Block != "" {
		s += " " + r.Block
	}
	if !r.Edition.IsZero() {
		k, _ := r.Edition.MarshalText()
		s += " (" + string(k) + ")"
	}
	return s
}

// appliedWordings are the words each applied operation wrote, by document:
// the content a set_content gave and the replacement text of each
// replace_text edit. They are what an agent's edit is held to when it follows
// a suggestion (noteAgentEdits).
func appliedWordings(set change.Set, res *change.Result) map[string][]string {
	out := map[string][]string{}
	for _, op := range res.Ops {
		if op.Status != change.OpApplied || op.I >= len(set.Ops) || op.At == nil {
			continue
		}
		switch body := set.Ops[op.I].Body.(type) {
		case *change.SetContent:
			if body.Text != nil {
				out[op.At.Doc] = append(out[op.At.Doc], *body.Text)
			}
		case *change.ReplaceText:
			for _, e := range body.Edits {
				out[op.At.Doc] = append(out[op.At.Doc], e.Text)
			}
		}
	}
	return out
}

// retiredChangeShape refuses a change set written in the entry shape kapi
// apply read before kapi.change/v1, naming the operation that takes each
// entry's place.
func retiredChangeShape(data []byte) error {
	kind, ok := retiredKind(data)
	if !ok {
		return nil
	}
	if kind == string(retiredVoiceKind) {
		return errRetiredVoiceKind
	}
	instead := map[string]string{
		"content": `{"op": "set_content", "at": {"doc": "docs/guide.md", "block": "<block key>"}, "if_match": "<rev>", "text": "..."}, ` +
			`or replace_text with "edits": [{"find": "...", "text": "..."}]`,
		"comment": `{"op": "set_content", "at": {"doc": "parse.go", "block": "func/Parse"}, "if_match": "<rev>", "text": "..."}`,
		"review":  `{"op": "decide", "at": {"doc": "docs/guide.md", "block": "<block key>", "edition": "fr"}, "if_match": "<rev>", "outcome": "establish"}`,
		"term":    `{"op": "term", "action": "upsert", "term": "...", "status": "preferred", "replaces": "..."}`,
		"memory":  `{"op": "memory", "action": "add", "from": {"edition": "en", "text": "..."}, "to": {"edition": "fr", "text": "..."}}`,
		"recipe":  `{"op": "recipe", "path": "...", "value": ...}`,
	}[kind]
	if instead == "" {
		instead = `{"op": "set_content", "at": {"doc": "...", "block": "..."}, "if_match": "<rev>", "text": "..."}`
	}
	return fmt.Errorf(`apply: this change set uses the retired entry shape (a "kind": %q entry with "file", "id" and "content_hash"); `+
		`kapi apply reads kapi.change/v1, whose operations name what they change with "op" and "at" and the revision they read with "if_match", `+
		`as kapi inspect prints them. Write this entry as %s; kapi apply --schema prints the whole contract`, kind, instead)
}

// retiredKind finds the kind of the first entry when data is in the retired
// entry shape: an object, or the first object of an array, with a "kind"
// field and no "ops". No kapi.change/v1 operation or envelope has a "kind"
// field; a retired asset entry carried its action in "op".
func retiredKind(data []byte) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var first json.RawMessage
	if err := dec.Decode(&first); err != nil {
		return "", false
	}
	first = bytes.TrimSpace(first)
	if len(first) > 0 && first[0] == '[' {
		var items []json.RawMessage
		if json.Unmarshal(first, &items) != nil || len(items) == 0 {
			return "", false
		}
		first = items[0]
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(first, &obj) != nil {
		return "", false
	}
	_, hasOps := obj["ops"]
	raw, hasKind := obj["kind"]
	if !hasKind || hasOps {
		return "", false
	}
	var kind string
	if json.Unmarshal(raw, &kind) != nil {
		return "", false
	}
	return kind, true
}
