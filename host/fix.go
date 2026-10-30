package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
// when every fix landed, 1 when a fix was not, and as kapi check does when the
// check did not run. When writing stops on an error, the files written before
// it are listed first.
func (a *App) RunFix(cmd Command, opts FixOptions) error {
	res, err := a.FixFiles(cmd, opts)
	if err != nil {
		if res != nil && len(res.Files) > 0 {
			if perr := output.Print(cmd, res); perr != nil {
				return errors.Join(err, perr)
			}
		}
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
// other fixes of the document land. A document the service cannot write
// without changing bytes outside the fixed words is not written. A finding
// that only reports is left as it is. After writing, the files written are
// checked again.
//
// When writing stops on an error, FixFiles returns the error with the result
// so far, which lists the files already written.
func (a *App) FixFiles(cmd Command, opts FixOptions) (*FixResult, error) {
	report, err := a.ComputeCheck(cmd, opts.Paths)
	if err != nil {
		return nil, err
	}
	if report.Verdict == check.VerdictDidNotRun {
		return nil, fmt.Errorf("%w: %s (%s): %s", ErrCheckNotRun, check.CauseSummary(report.DidNotRunCause), report.DidNotRunCause, strings.Join(report.DidNotRun, "; "))
	}
	return a.fixReport(cmd, opts, report)
}

// fixReport applies the fixes of a report kapi fix's check produced.
func (a *App) fixReport(cmd Command, opts FixOptions, report check.Report) (*FixResult, error) {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return nil, err
	}
	res := &FixResult{Schema: FixResultSchemaID, DryRun: opts.DryRun, Files: []FixedFile{}, NotFixed: []UnfixedFinding{}, NeedsRewrite: []check.Diagnostic{}}
	docs := a.newFixDocs(recipe)
	for _, d := range report.Findings {
		if !d.Fails {
			continue
		}
		switch {
		case d.Fix != nil:
		case docs.translation(d.Location.File):
			res.NotFixed = append(res.NotFixed, UnfixedFinding{Finding: d, Reason: fixReasonTranslation})
		default:
			res.NeedsRewrite = append(res.NeedsRewrite, d)
		}
	}
	byDoc, order := fixesByDoc(report.Findings)
	if len(order) > 0 {
		if err := a.applyFixes(cmd, recipe, opts, byDoc, order, res); err != nil {
			return res, err
		}
	}
	if !opts.DryRun {
		remaining, err := a.fixRemaining(cmd, opts, report, res)
		if err != nil {
			return res, err
		}
		res.Remaining = remaining
	}
	return res, nil
}

// The reasons kapi fix gives for a fix it did not make, beside the change
// service's refusals (fixRefusalReason).
const (
	fixReasonTranslation = "a translation's file; edit the source or review it in kapi status --review"
	fixReasonOtherBytes  = "kapi cannot write this file without changing other bytes"
)

// translation reports whether file is the file of a translation in the
// project, which no fix names (fixDocs.names).
func (d *fixDocs) translation(file string) bool {
	if d.recipe == "" || file == "" {
		return false
	}
	if _, named := d.doc(file); named {
		return false
	}
	return d.layout != nil
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

// fixer applies the fixes of one kapi fix run, document by document.
type fixer struct {
	app      *App
	cmd      Command
	recipe   string
	root     string
	actor    change.Actor
	resolved ResolvedActor
	note     string
	services map[bool]*change.Service
}

// service is the change service for a document inside the project, or for
// one outside it, which resolves from the project's root as kapi apply
// resolves one.
func (f *fixer) service(outside bool) (*change.Service, error) {
	if svc, ok := f.services[outside]; ok {
		return svc, nil
	}
	opts := ChangeServiceOptions{
		Project: f.recipe, Origin: "fix", Encoding: f.app.Encoding, SourceLocale: model.LocaleID(f.app.SourceLang),
		AnyPath: f.recipe == "",
	}
	if outside {
		opts.Project, opts.Root, opts.AnyPath = "", f.root, true
	}
	svc, err := f.app.changeService(CmdContext(f.cmd), f.cmd, opts)
	if err != nil {
		return nil, err
	}
	f.services[outside] = svc
	return svc, nil
}

// applyFixes applies each document's fixes as one change set. Each set is
// previewed first. A previewed set with a refused operation writes nothing, so
// the refused operation is taken out, recorded as not fixed, and the rest is
// previewed again. A preview whose file differs from the file on disk
// anywhere but in the words the fixes replace is not applied, and every fix of
// the document is recorded as not fixed: kapi fix writes only the words a
// finding names.
func (a *App) applyFixes(cmd Command, recipe string, opts FixOptions, docs map[string]*docFixes, order []string, res *FixResult) error {
	resolved, err := a.commandActor()
	if err != nil {
		return err
	}
	root, err := changeRoot(recipe)
	if err != nil {
		return err
	}
	f := &fixer{app: a, cmd: cmd, recipe: recipe, root: root, actor: changeActorOf(resolved.Actor), resolved: resolved,
		note: resolved.NoteWith("applied with `kapi fix`"), services: map[bool]*change.Service{}}
	for _, doc := range order {
		if err := f.fixDoc(doc, docs[doc], opts.DryRun, res); err != nil {
			return err
		}
	}
	return nil
}

// fixDoc previews, guards and applies the fixes of one document.
func (f *fixer) fixDoc(doc string, df *docFixes, dryRun bool, res *FixResult) error {
	ctx := CmdContext(f.cmd)
	outside := f.recipe != "" && filepath.IsAbs(filepath.FromSlash(doc)) && !under(f.root, filepath.FromSlash(doc))
	svc, err := f.service(outside)
	if err != nil {
		return err
	}
	ops := slices.Clone(df.ops)
	for len(ops) > 0 {
		set := change.Set{Mode: change.ModePreview, Note: f.note, Ops: make([]change.Op, len(ops))}
		for i, o := range ops {
			set.Ops[i] = o.op
		}
		original, rerr := os.ReadFile(df.file)
		preview, err := svc.Apply(ctx, set, f.actor)
		if err != nil {
			return err
		}
		if preview.Status != change.SetPreviewed {
			ops = dropRefused(ops, preview, res)
			continue
		}
		if rerr != nil || !keepsOtherBytes(original, ops, preview) {
			for _, o := range ops {
				res.NotFixed = append(res.NotFixed, unfixedFor(o, fixReasonOtherBytes, nil)...)
			}
			return nil
		}
		if dryRun {
			f.landed(df, doc, ops, preview, res)
			return nil
		}
		set.Mode = change.ModeApply
		var before map[int]string
		if f.actor.Kind == change.ActorAgent {
			before = wordingBefore(ctx, svc, set)
		}
		out, err := svc.Apply(ctx, set, f.actor)
		if err != nil {
			return err
		}
		switch out.Status {
		case change.SetApplied, change.SetPartial:
			f.app.noteAgentEdits(ctx, f.recipe, f.resolved.Actor, appliedWording(set, out, before))
			f.landed(df, doc, ops, out, res)
			return nil
		}
		ops = dropRefused(ops, out, res)
	}
	return nil
}

// landed records a document's change set that landed, or would in a dry run.
func (f *fixer) landed(df *docFixes, doc string, ops []fixOp, out *change.Result, res *FixResult) {
	res.Files = append(res.Files, fixedFile(df.file, doc, ops, out))
	res.Fixed += res.Files[len(res.Files)-1].Fixed
}

// dropRefused records the operation that refused a change set as not fixed
// and returns the others. A change set refused as a whole fixes nothing.
func dropRefused(ops []fixOp, out *change.Result, res *FixResult) []fixOp {
	i := refusedOp(out)
	if i < 0 || i >= len(ops) {
		for _, o := range ops {
			res.NotFixed = append(res.NotFixed, unfixed(o, out.Error)...)
		}
		return nil
	}
	res.NotFixed = append(res.NotFixed, unfixed(ops[i], out.Ops[i].Error)...)
	return slices.Delete(ops, i, i+1)
}

// keepsOtherBytes reports whether the file a preview would write differs from
// original only in the words the previewed fixes replace. The file is rebuilt
// from original and the preview's diff, and must have the digest the preview
// reports. A preview that writes nothing keeps every byte; one that writes
// more than one file, renders no diff (a file too large or not text) or
// cannot be rebuilt is not shown to keep them.
func keepsOtherBytes(original []byte, ops []fixOp, preview *change.Result) bool {
	var written []change.DocResult
	for _, d := range preview.Docs {
		if d.After != nil {
			written = append(written, d)
		}
	}
	if len(written) == 0 {
		return true
	}
	if len(written) > 1 || len(preview.Docs) == 0 || preview.Docs[0].Diff == "" {
		return false
	}
	after, ok := applyUnifiedDiff(string(original), preview.Docs[0].Diff)
	if !ok {
		return false
	}
	sum := sha256.Sum256([]byte(after))
	if "sha256:"+hex.EncodeToString(sum[:]) != *written[0].After {
		return false
	}
	var pairs []FixPair
	for i, o := range ops {
		if i < len(preview.Ops) && preview.Ops[i].Status != change.OpPreviewed {
			continue
		}
		body, ok := o.op.Body.(*change.ReplaceText)
		if !ok || len(body.Edits) != 1 || len(o.findings) == 0 {
			return false
		}
		pairs = append(pairs, FixPair{From: o.findings[0].Location.Snippet, To: body.Edits[0].Text})
	}
	return onlyPairsDiffer(string(original), after, pairs)
}

// onlyPairsDiffer reports whether after is before with each pair's From
// replaced by its To once, in some order, and nothing else changed. At each
// point where the two texts part, the pair that accounts for the difference is
// the one whose words start closest to that point.
func onlyPairsDiffer(before, after string, pairs []FixPair) bool {
	i, j := 0, 0
	left := slices.Clone(pairs)
	for len(left) > 0 {
		m := commonPrefix(before[i:], after[j:])
		pick, at := -1, -1
		for n, p := range left {
			if p.From == "" {
				return false
			}
			for k := i + m; k >= i; k-- {
				if strings.HasPrefix(before[k:], p.From) && strings.HasPrefix(after[j+(k-i):], p.To) {
					if k > at {
						pick, at = n, k
					}
					break
				}
			}
		}
		if pick < 0 {
			return false
		}
		p := left[pick]
		j += at - i + len(p.To)
		i = at + len(p.From)
		left = slices.Delete(left, pick, pick+1)
	}
	return before[i:] == after[j:]
}

// commonPrefix is the length in bytes of the prefix a and b share.
func commonPrefix(a, b string) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// applyUnifiedDiff applies a unified diff of one file, as a change preview
// renders it, to before. The preview splits both texts into lines that each
// end in a newline, the last one given a newline it may not have, so the
// result drops that one. False when the diff does not apply to before.
func applyUnifiedDiff(before, diff string) (string, bool) {
	lines := previewLines(before)
	var out []string
	pos := 0
	inHunk := false
	for _, l := range strings.SplitAfter(diff, "\n") {
		switch {
		case l == "":
		case strings.HasPrefix(l, "@@ "):
			start, ok := hunkStart(l)
			if !ok || start < pos || start > len(lines) {
				return "", false
			}
			out = append(out, lines[pos:start]...)
			pos, inHunk = start, true
		case !inHunk:
			// The --- and +++ header lines.
		case l[0] == ' ' || l[0] == '-':
			if pos >= len(lines) || lines[pos] != l[1:] {
				return "", false
			}
			if l[0] == ' ' {
				out = append(out, lines[pos])
			}
			pos++
		case l[0] == '+':
			out = append(out, l[1:])
		default:
			return "", false
		}
	}
	out = append(out, lines[pos:]...)
	return strings.TrimSuffix(strings.Join(out, ""), "\n"), true
}

// previewLines splits s the way the preview's diff does: after each newline,
// with a newline added to the last line.
func previewLines(s string) []string {
	lines := strings.SplitAfter(s, "\n")
	lines[len(lines)-1] += "\n"
	return lines
}

// hunkStart is the index of the first line of before a hunk header
// ("@@ -start,count +start,count @@") names. A hunk of no lines names the
// line before it.
func hunkStart(header string) (int, bool) {
	from, _, ok := strings.Cut(strings.TrimPrefix(header, "@@ -"), " ")
	if !ok {
		return 0, false
	}
	first, count, hasCount := strings.Cut(from, ",")
	n, err := strconv.Atoi(first)
	if err != nil {
		return 0, false
	}
	if hasCount {
		c, err := strconv.Atoi(count)
		if err != nil {
			return 0, false
		}
		if c == 0 {
			return n, true
		}
	}
	return n - 1, true
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
	return unfixedFor(o, fixRefusalReason(e), e)
}

// unfixedFor records the findings of an operation that did not land as not
// fixed, for reason.
func unfixedFor(o fixOp, reason string, e *change.Error) []UnfixedFinding {
	out := make([]UnfixedFinding, 0, len(o.findings))
	for _, d := range o.findings {
		out = append(out, UnfixedFinding{Finding: d, Reason: reason, Error: e})
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
		// The revision the check computed is not the one the service reads:
		// the file changed since, or the service reads the block another
		// way.
		return "kapi read this block differently when writing it"
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
	case r.Fixed == 0 && len(r.NotFixed) == 0 && len(r.NeedsRewrite) == 0:
		fmt.Fprintln(w, "Nothing to fix: no failing findings.")
	case r.Fixed == 0 && len(r.NotFixed) == 0:
		fmt.Fprintln(w, "Nothing to fix: no failing finding has an automatic fix.")
	case r.DryRun:
		fmt.Fprintf(w, "%d %s %s %s; nothing written (--dry-run)\n", r.Fixed, fixed, inFiles, pluralUnit(len(r.Files), "file", "files"))
	default:
		fmt.Fprintf(w, "%d %s %s %s\n", r.Fixed, fixed, inFiles, pluralUnit(len(r.Files), "file", "files"))
	}
	// A file is named as its change set names it: by its path from the
	// project root inside a project, whichever directory kapi fix ran in.
	width := 0
	for _, f := range r.Files {
		width = max(width, len(f.Doc))
	}
	for _, f := range r.Files {
		fmt.Fprintf(w, "  %-*s  %d %s", width, f.Doc, f.Fixed, fixed)
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
			if u.Finding.Fix != nil {
				loc = u.Finding.Fix.At.Doc
			}
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
