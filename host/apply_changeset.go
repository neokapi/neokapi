package host

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/changeschema"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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
// refused (nothing is written), the change landed in part, or, under the
// enforcing gate, the check of a code comment it wrote found a failing
// finding, and 5 when a backend did not answer.
func (a *App) RunApply(cmd Command, path string, opts ApplyOptions) error {
	ctx := cmd.Context()
	data, err := readContent(ctx, path)
	if err != nil {
		return err
	}
	if err := retiredChangeShape(data); err != nil {
		return a.refuseUndecodable(cmd, opts, &change.Error{Code: change.CodeInvalid, Message: err.Error()}, err)
	}
	set, err := change.Decode(bytes.NewReader(data))
	if err != nil {
		ce, ok := errors.AsType[*change.Error](err)
		if !ok || ce == nil {
			ce = &change.Error{Code: change.CodeInvalid, Message: err.Error()}
		}
		return a.refuseUndecodable(cmd, opts, ce, fmt.Errorf("apply: %w", err))
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
	if line := toolOriginNote(set, actor); line != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), line)
	}
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

	comments, err := a.commentSet(set, root, a.newCommentDocs(recipe))
	if err != nil {
		return WithExitCode(ExitUsage, err)
	}
	// A document named by its absolute path outside the project is a file
	// kapi inspect read outside it, edited as a file outside any project is.
	outside, err := outsideProject(set, recipe)
	if err != nil {
		return WithExitCode(ExitUsage, err)
	}
	var res *change.Result
	if comments {
		res, _, err = a.applyCommentChange(cmd, set, root, opts.BackupSuffix, path == "" || path == StdinName)
	} else {
		svcOpts := ChangeServiceOptions{
			Project: recipe, Origin: "apply", Format: a.FormatFlag, SourceLocale: model.LocaleID(a.SourceLang),
			AnyPath: recipe == "", BackupSuffix: opts.BackupSuffix,
			TargetLocale: targetLocaleOf(set, a.changeSourceLocale(recipe)),
		}
		if outside {
			// Resolved from the project's root, under which none of the files
			// lies, so each keeps its absolute path in the result.
			svcOpts.Project, svcOpts.Root, svcOpts.AnyPath = "", root, true
		}
		var svc *change.Service
		svc, err = a.changeService(ctx, cmd, svcOpts)
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
	if comments {
		return commentSetExit(set, res)
	}
	return changeResultExit(res)
}

// toolOriginNote is the one line kapi apply prints when operations of a
// change set state how a tool produced their content, as a run printed with
// --print-ops states it: the change is recorded as the sender's edit, so the
// tool's origin is not kept. Empty when no operation states one.
func toolOriginNote(set change.Set, actor change.Actor) string {
	var tools []string
	n := 0
	for _, op := range set.Ops {
		var o *change.ToolOrigin
		switch body := op.Body.(type) {
		case *change.SetContent:
			o = body.Origin
		case *change.ReplaceText:
			o = body.Origin
		}
		if o == nil {
			continue
		}
		n++
		if !slices.Contains(tools, o.Tool) {
			tools = append(tools, o.Tool)
		}
	}
	if n == 0 {
		return ""
	}
	whose := "yours"
	if actor.Kind == change.ActorAgent {
		whose = "the agent's"
	}
	what := "1 operation states how %s produced its content"
	if n > 1 {
		what = fmt.Sprintf("%d operations state how %%s produced their content", n)
	}
	return fmt.Sprintf("note: "+what+"; kapi apply records the change as %s, and that origin is not kept",
		strings.Join(tools, ", "), whose)
}

// refuseUndecodable answers a change set that does not decode, exit 2: with
// --json, the refused kapi.change-result/v1 whose error says why, as MCP and
// the browser answer it, on standard output; otherwise err, which the command
// line prints.
func (a *App) refuseUndecodable(cmd Command, opts ApplyOptions, ce *change.Error, err error) error {
	if !opts.JSON {
		return WithExitCode(ExitUsage, err)
	}
	if werr := writeChangeResult(cmd.OutOrStdout(), change.ErrorResult(ce)); werr != nil {
		return werr
	}
	return WithExitCode(ExitUsage, ErrSilentExit)
}

// outsideProject reports whether a change set applied in the project at
// recipe edits files outside it: every document it names is an absolute path
// outside the project's root, as kapi inspect names a file it read outside
// the project. A change set that edits such a file and a document of the
// project, or writes to the project's stores, is refused.
func outsideProject(set change.Set, recipe string) (bool, error) {
	if recipe == "" {
		return false, nil
	}
	root := filepath.Dir(recipe)
	var outside, inside []string
	for _, op := range set.Ops {
		doc, _ := splitLocator(op.At.Doc)
		switch {
		case op.At.Doc == "":
			inside = append(inside, string(op.Kind)+" operation")
		case filepath.IsAbs(filepath.FromSlash(doc)) && !under(root, filepath.FromSlash(doc)):
			outside = append(outside, op.At.Doc)
		default:
			inside = append(inside, op.At.Doc)
		}
	}
	switch {
	case len(outside) == 0:
		return false, nil
	case len(inside) > 0:
		return false, fmt.Errorf("apply: this change set edits %s, outside the project at %s, and %s of the project; "+
			"send the edits of files outside the project in a change set of their own", outside[0], root, inside[0])
	}
	return true, nil
}

// under reports whether path lies inside root.
func under(root, path string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(path))
	return err == nil && filepath.IsLocal(rel)
}

// targetLocaleOf is the one language a change set's operations name as an
// edition other than source, or "" when they name none or several. A
// bilingual file whose reader has to be told the language of the translation
// it holds (a PO catalog's msgstr) is read in that language.
func targetLocaleOf(set change.Set, source model.LocaleID) model.LocaleID {
	return SoleTargetLocale(OpEditions(set), source)
}

// SoleTargetLocale is the one language editions name other than source, or
// "" when they name none or several. A change service built for a call that
// names editions reads a bilingual file in it (ChangeServiceOptions.
// TargetLocale), as kapi apply, MCP, the browser and Kapi Desktop do.
func SoleTargetLocale(editions []model.EditionKey, source model.LocaleID) model.LocaleID {
	var loc model.LocaleID
	for _, e := range editions {
		l := model.NormalizeLocale(e.Locale)
		if l == "" || l == model.NormalizeLocale(source) {
			continue
		}
		if loc != "" && loc != l {
			return ""
		}
		loc = l
	}
	return loc
}

// changeSourceLocale is the language the documents of the project at recipe
// are written in, or the command line's outside a project.
func (a *App) changeSourceLocale(recipe string) model.LocaleID {
	if recipe != "" {
		if proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true}); err == nil {
			return model.LocaleID(ResolveSourceLocale(a.SourceLang, proj.Defaults.SourceLanguage))
		}
	}
	return model.LocaleID(a.SourceLocale())
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
	if len(res.Ops) == 0 {
		parts = append(parts, "no operation, nothing to change")
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
	shape, ok := retiredShapes[kind]
	if !ok {
		shape = retiredShape{
			fields:  `"file", "id" and "content_hash"`,
			instead: `{"op": "set_content", "at": {"doc": "...", "block": "..."}, "if_match": "<rev>", "text": "..."}`,
		}
	}
	return fmt.Errorf(`apply: this change set uses the retired entry shape (a "kind": %q entry with %s); `+
		`kapi apply reads kapi.change/v1, whose operations name what they change with "op" and "at" and the revision they read with "if_match", `+
		`as kapi inspect prints them. Write this entry as %s; kapi apply --schema prints the whole contract`, kind, shape.fields, shape.instead)
}

// retiredShape is what an entry of the retired shape carried, and the
// operation that takes its place.
type retiredShape struct{ fields, instead string }

// retiredShapes describes each kind of the retired entry shape.
var retiredShapes = map[string]retiredShape{
	"content": {`"file", "id" and "content_hash"`,
		`{"op": "set_content", "at": {"doc": "docs/guide.md", "block": "<block key>"}, "if_match": "<rev>", "text": "..."}, ` +
			`or replace_text with "edits": [{"find": "...", "text": "..."}]`},
	"comment": {`"file", "id" and "comment_sha256"`,
		`{"op": "set_content", "at": {"doc": "parse.go", "block": "func/Parse"}, "if_match": "<rev>", "text": "..."}`},
	"review": {`"file", "id", "locale" and "status"`,
		`{"op": "decide", "at": {"doc": "docs/guide.md", "block": "<block key>", "edition": "fr"}, "if_match": "<rev>", "outcome": "establish"}`},
	"term": {`"op", "term" and "status"`,
		`{"op": "term", "action": "upsert", "term": "...", "status": "preferred", "replaces": "..."}`},
	"memory": {`"op", "source" and "target"`,
		`{"op": "memory", "action": "add", "from": {"edition": "en", "text": "..."}, "to": {"edition": "fr", "text": "..."}}`},
	"recipe": {`"op", "path" and "value"`,
		`{"op": "recipe", "path": "...", "value": ...}`},
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
