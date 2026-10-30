package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/output"
)

// FixResultSchemaID names the shape of a FixResult on the wire.
const FixResultSchemaID = "kapi.fix-result/v1"

// FixOptions are the options of one kapi fix run.
type FixOptions struct {
	// Paths are the files, directories or globs to check and fix. Empty, inside
	// a project, is the project's declared content, as a bare kapi check reads
	// it.
	Paths []string
	// DryRun computes and checks every change set and writes nothing.
	DryRun bool
}

// FixResult is what kapi fix did: the fixes it applied, file by file, the
// fixes the change service refused, the failing findings that have no fix,
// and the failing findings a check of the content finds afterwards.
type FixResult struct {
	Schema string `json:"schema"`
	// DryRun reports a run that wrote nothing.
	DryRun bool `json:"dry_run,omitempty"`
	// Fixed counts the findings whose fix landed, or would land in a dry run.
	Fixed int `json:"fixed"`
	// Files lists each file a fix landed in, in the order the check read
	// them.
	Files []FixedFile `json:"files"`
	// NotFixed are the findings whose fix the change service refused, each
	// with the refusal.
	NotFixed []UnfixedFinding `json:"not_fixed"`
	// NeedsRewrite are the failing findings that carry no fix.
	NeedsRewrite []check.Diagnostic `json:"needs_rewrite"`
	// Remaining counts the failing findings left after the fixes, nil in a dry
	// run.
	Remaining *FixRemaining `json:"remaining,omitempty"`
}

// FixedFile is one file kapi fix changed.
type FixedFile struct {
	// File is the file as the check named it.
	File string `json:"file"`
	// Doc is the document the change set named.
	Doc string `json:"doc"`
	// Fixed counts the findings fixed in the file.
	Fixed int `json:"fixed"`
	// Pairs are the words replaced and their replacements, one per fix.
	Pairs []FixPair `json:"pairs"`
	// Result is the change service's result for the file's change set
	// (kapi.change-result/v1).
	Result *change.Result `json:"result"`
}

// FixPair is one replacement a fix made.
type FixPair struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// UnfixedFinding is a finding whose fix the change service refused.
type UnfixedFinding struct {
	Finding check.Diagnostic `json:"finding"`
	// Reason says in one clause why the fix did not land.
	Reason string `json:"reason"`
	// Error is the refusal as the change service returned it.
	Error *change.Error `json:"error,omitempty"`
}

// FixRemaining counts the failing findings a check finds after the fixes:
// those in the files kapi fix wrote, checked again, and those in the files it
// left alone.
type FixRemaining struct {
	Failing int `json:"failing"`
	// Checks are the check families the remaining failing findings belong to.
	Checks []string `json:"checks,omitempty"`
}

// RunFix is kapi fix: it runs the check, applies every fix the check
// computed through the change service, and prints what it did. It exits 0
// when every fix landed, 1 when a fix was refused, and as kapi check does
// when the check did not run.
func (a *App) RunFix(cmd Command, opts FixOptions) error {
	res, err := a.FixFiles(CmdContext(cmd), cmd, opts)
	if err != nil {
		return err
	}
	if output.ResolveFormat(cmd) == output.FormatText && opts.DryRun {
		for _, f := range res.Files {
			for _, d := range f.Result.Docs {
				fmt.Fprint(cmd.OutOrStdout(), d.Diff)
			}
		}
	}
	if err := output.Print(cmd, res); err != nil {
		return err
	}
	if len(res.NotFixed) > 0 {
		return ErrSilentExit
	}
	return nil
}

// FixFiles runs the check kapi check runs over opts.Paths (the project's
// declared content when none is given), and applies the fix of every failing
// finding that carries one, one change set per document, through the change
// service kapi apply uses, as the person or agent the environment names. A fix
// the service refuses is left out of its change set and reported, and the
// other fixes of the document land. A finding that only reports is left as it
// is. After writing, the files written are checked again.
func (a *App) FixFiles(ctx context.Context, cmd Command, opts FixOptions) (*FixResult, error) {
	report, err := a.ComputeCheck(cmd, opts.Paths)
	if err != nil {
		return nil, err
	}
	if report.Verdict == check.VerdictDidNotRun {
		return nil, fmt.Errorf("%w: %s (%s): %s", ErrCheckNotRun, check.CauseSummary(report.DidNotRunCause), report.DidNotRunCause, strings.Join(report.DidNotRun, "; "))
	}

	res := &FixResult{Schema: FixResultSchemaID, DryRun: opts.DryRun, Files: []FixedFile{}, NotFixed: []UnfixedFinding{}, NeedsRewrite: []check.Diagnostic{}}
	docs, order := fixesByDoc(report.Findings)
	for _, d := range report.Findings {
		if d.Fails && d.Fix == nil {
			res.NeedsRewrite = append(res.NeedsRewrite, d)
		}
	}
	if len(order) > 0 {
		if err := a.applyFixes(ctx, cmd, opts, docs, order, res); err != nil {
			return nil, err
		}
	}
	if !opts.DryRun {
		remaining, err := a.fixRemaining(cmd, opts, report, res)
		if err != nil {
			return nil, err
		}
		res.Remaining = remaining
	}
	return res, nil
}

// docFixes are the fixes of one document: its operations, each with the
// findings it fixes, and the file the check read it from.
type docFixes struct {
	file string
	ops  []fixOp
}

// fixOp is one operation and the findings it fixes. Two findings can carry
// the same operation (one word under two rules), and it applies once.
type fixOp struct {
	op       change.Op
	findings []check.Diagnostic
}

// fixesByDoc groups the fixes of the failing findings by the document they
// name, in the order the check reported them.
func fixesByDoc(findings []check.Diagnostic) (map[string]*docFixes, []string) {
	docs := map[string]*docFixes{}
	var order []string
	seen := map[string]int{}
	for _, d := range findings {
		if !d.Fails || d.Fix == nil {
			continue
		}
		doc := d.Fix.At.Doc
		df, ok := docs[doc]
		if !ok {
			df = &docFixes{file: d.Location.File}
			docs[doc] = df
			order = append(order, doc)
		}
		key := doc + "\x00" + opKey(*d.Fix)
		if i, dup := seen[key]; dup {
			df.ops[i].findings = append(df.ops[i].findings, d)
			continue
		}
		seen[key] = len(df.ops)
		df.ops = append(df.ops, fixOp{op: *d.Fix, findings: []check.Diagnostic{d}})
	}
	return docs, order
}

// opKey identifies an operation by its wire form.
func opKey(op change.Op) string {
	raw, err := json.Marshal(op)
	if err != nil {
		return fmt.Sprintf("%p", &op)
	}
	return string(raw)
}

// applyFixes applies each document's fixes as one change set. A change set
// with a refused operation writes nothing, so the refused operation is taken
// out, recorded as not fixed, and the rest is sent again.
func (a *App) applyFixes(ctx context.Context, cmd Command, opts FixOptions, docs map[string]*docFixes, order []string, res *FixResult) error {
	resolved, err := a.commandActor()
	if err != nil {
		return err
	}
	actor := changeActorOf(resolved.Actor)
	note := resolved.NoteWith("applied with `kapi fix`")
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	root, err := changeRoot(recipe)
	if err != nil {
		return err
	}
	services := map[bool]*change.Service{}
	serviceFor := func(outside bool) (*change.Service, error) {
		if svc, ok := services[outside]; ok {
			return svc, nil
		}
		svcOpts := ChangeServiceOptions{
			Project: recipe, Origin: "fix", Encoding: a.Encoding, SourceLocale: model.LocaleID(a.SourceLang),
			AnyPath: recipe == "",
		}
		if outside {
			svcOpts.Project, svcOpts.Root, svcOpts.AnyPath = "", root, true
		}
		svc, err := a.changeService(ctx, cmd, svcOpts)
		if err != nil {
			return nil, err
		}
		services[outside] = svc
		return svc, nil
	}
	mode := change.ModeApply
	if opts.DryRun {
		mode = change.ModePreview
	}
	for _, doc := range order {
		df := docs[doc]
		outside := recipe != "" && filepath.IsAbs(filepath.FromSlash(doc)) && !under(root, filepath.FromSlash(doc))
		svc, err := serviceFor(outside)
		if err != nil {
			return err
		}
		ops := slices.Clone(df.ops)
		for len(ops) > 0 {
			set := change.Set{Mode: mode, Note: note, Ops: make([]change.Op, len(ops))}
			for i, o := range ops {
				set.Ops[i] = o.op
			}
			var before map[int]string
			if actor.Kind == change.ActorAgent {
				before = wordingBefore(ctx, svc, set)
			}
			out, err := svc.Apply(ctx, set, actor)
			if err != nil {
				return err
			}
			switch out.Status {
			case change.SetApplied, change.SetPreviewed, change.SetPartial:
				if out.Status != change.SetPreviewed {
					a.noteAgentEdits(ctx, recipe, resolved.Actor, appliedWording(set, out, before))
				}
				res.Files = append(res.Files, fixedFile(df.file, doc, ops, out))
				res.Fixed += res.Files[len(res.Files)-1].Fixed
				ops = nil
				continue
			}
			i := refusedOp(out)
			if i < 0 {
				// Refused as a whole: none of the document's fixes can land.
				for _, o := range ops {
					res.NotFixed = append(res.NotFixed, unfixed(o, out.Error)...)
				}
				break
			}
			res.NotFixed = append(res.NotFixed, unfixed(ops[i], out.Ops[i].Error)...)
			ops = slices.Delete(ops, i, i+1)
		}
	}
	return nil
}

// refusedOp is the index of the operation that refused a change set, or -1
// when the change set was refused as a whole.
func refusedOp(res *change.Result) int {
	for _, op := range res.Ops {
		if op.Status == change.OpRefused {
			return op.I
		}
	}
	return -1
}

// fixedFile is the account of one document's change set that landed.
func fixedFile(file, doc string, ops []fixOp, res *change.Result) FixedFile {
	f := FixedFile{File: file, Doc: doc, Result: res, Pairs: []FixPair{}}
	for i, o := range ops {
		if i < len(res.Ops) && res.Ops[i].Status == change.OpNotApplied {
			continue
		}
		f.Fixed += len(o.findings)
		to := ""
		if body, ok := o.op.Body.(*change.ReplaceText); ok && len(body.Edits) > 0 {
			to = body.Edits[0].Text
		}
		for _, d := range o.findings {
			f.Pairs = append(f.Pairs, FixPair{From: d.Location.Snippet, To: to})
		}
	}
	for _, d := range res.Docs {
		if d.File != "" && file == "" {
			f.File = d.File
		}
	}
	return f
}

// unfixed records the findings of a refused operation as not fixed.
func unfixed(o fixOp, e *change.Error) []UnfixedFinding {
	out := make([]UnfixedFinding, 0, len(o.findings))
	for _, d := range o.findings {
		out = append(out, UnfixedFinding{Finding: d, Reason: fixRefusalReason(e), Error: e})
	}
	return out
}

// fixRefusalReason says in one clause why the change service refused a fix.
func fixRefusalReason(e *change.Error) string {
	if e == nil {
		return "the change was refused"
	}
	switch {
	case e.Subcode == change.SubcodeOverlap:
		return "another fix changes the same words"
	case e.Code == change.CodeAmbiguous:
		return "the words occur more than once in the block"
	case e.Code == change.CodeStale, e.Code == change.CodeDocChanged:
		return "the block changed after the check read it"
	case e.Code == change.CodeGuard:
		return "the replacement would remove formatting inside the words"
	case e.Code == change.CodeGateFailed:
		return "the replacement fails a rule of its own"
	}
	return strings.TrimSuffix(e.Message, ".")
}

// fixRemaining counts the failing findings after the fixes: the files kapi
// fix wrote are checked again, and the findings of every other file stand as
// the first check found them.
func (a *App) fixRemaining(cmd Command, opts FixOptions, first check.Report, res *FixResult) (*FixRemaining, error) {
	touched := map[string]bool{}
	var files []string
	for _, f := range res.Files {
		if !fileWritten(f.Result) || touched[f.File] {
			continue
		}
		touched[f.File] = true
		files = append(files, f.File)
	}
	var failing []check.Diagnostic
	for _, d := range first.Findings {
		if d.Fails && !touched[d.Location.File] {
			failing = append(failing, d)
		}
	}
	if len(files) > 0 {
		var again check.Report
		var err error
		if len(opts.Paths) == 0 {
			again, err = a.ComputeDeclaredCheck(cmd, files)
		} else {
			again, err = a.ComputeCheck(cmd, files)
		}
		if err != nil {
			return nil, fmt.Errorf("check after the fixes: %w", err)
		}
		for _, d := range again.Findings {
			if d.Fails {
				failing = append(failing, d)
			}
		}
	}
	out := &FixRemaining{Failing: len(failing)}
	for _, d := range failing {
		if d.Check != "" && !slices.Contains(out.Checks, d.Check) {
			out.Checks = append(out.Checks, d.Check)
		}
	}
	return out, nil
}

// fileWritten reports whether a change set wrote a file.
func fileWritten(res *change.Result) bool {
	if res == nil {
		return false
	}
	return slices.ContainsFunc(res.Docs, func(d change.DocResult) bool { return d.Written })
}

// FormatText writes the result for a person: the count, a row per file, the
// findings left for a person to edit, and what a check finds afterwards.
func (r *FixResult) FormatText(w io.Writer) error {
	fixed, inFiles := "fixed", "in"
	if r.DryRun {
		fixed = "to fix"
	}
	switch {
	case r.Fixed == 0 && len(r.NotFixed) == 0:
		fmt.Fprintln(w, "Nothing to fix: no failing finding has an automatic fix.")
	case r.DryRun:
		fmt.Fprintf(w, "%d %s %s %s; nothing written (--dry-run)\n", r.Fixed, fixed, inFiles, pluralUnit(len(r.Files), "file", "files"))
	default:
		fmt.Fprintf(w, "%d %s %s %s\n", r.Fixed, fixed, inFiles, pluralUnit(len(r.Files), "file", "files"))
	}
	width := 0
	for _, f := range r.Files {
		width = max(width, len(f.File))
	}
	for _, f := range r.Files {
		fmt.Fprintf(w, "  %-*s  %d %s", width, f.File, f.Fixed, fixed)
		if len(f.Pairs) <= 3 {
			pairs := make([]string, len(f.Pairs))
			for i, p := range f.Pairs {
				pairs[i] = p.From + " → " + p.To
			}
			fmt.Fprintf(w, ": %s", strings.Join(pairs, ", "))
		}
		fmt.Fprintln(w)
	}
	if n := len(r.NeedsRewrite); n > 0 {
		var rules []string
		for _, d := range r.NeedsRewrite {
			if !slices.Contains(rules, d.Rule) {
				rules = append(rules, d.Rule)
			}
		}
		fmt.Fprintf(w, "%s %s a rewrite, with no automatic fix (%s). Edit by hand.\n",
			pluralUnit(n, "finding", "findings"), verbFor(n, "needs", "need"), strings.Join(rules, ", "))
	}
	if n := len(r.NotFixed); n > 0 {
		fmt.Fprintf(w, "%s not fixed. Edit by hand:\n", pluralUnit(n, "finding", "findings"))
		for _, u := range r.NotFixed {
			loc := u.Finding.Location.File
			if u.Finding.Location.Block != "" {
				loc += " " + u.Finding.Location.Block
			}
			fmt.Fprintf(w, "  %s %q: %s\n", loc, u.Finding.Location.Snippet, u.Reason)
		}
	}
	if r.Remaining != nil {
		if r.Remaining.Failing == 0 {
			fmt.Fprintln(w, "kapi check: no failing findings")
		} else {
			fmt.Fprintf(w, "kapi check: %s %s (%s)\n", pluralUnit(r.Remaining.Failing, "failing finding", "failing findings"),
				verbFor(r.Remaining.Failing, "remains", "remain"), strings.Join(r.Remaining.Checks, ", "))
		}
	}
	return nil
}

// verbFor picks the verb form that agrees with n.
func verbFor(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
