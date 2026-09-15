package host

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/diffscope"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/pmezard/go-difflib/difflib"
)

// rewriteComment rewrites one comment. A test replaces it to show that the
// write canary invalidates a run whose rewrite is not contained.
var rewriteComment comment.RewriteFunc = comment.Rewrite

// The outcomes of a comment entry.
const (
	commentWritten   = "written"
	commentUnchanged = "unchanged"
	commentRefused   = "refused"
	commentNotRun    = "did-not-run"
)

// The reasons a comment entry was refused or did not run, beside the reasons
// comment.RefusalReason names.
const (
	// reasonCanary is a run whose write canary failed for the file's language.
	reasonCanary = "canary"
	// reasonPreview is an edit `kapi apply --diff` shows and never writes.
	reasonPreview = "preview"
	// reasonDuplicate is a second entry for a comment the change-set already
	// rewrites.
	reasonDuplicate = "duplicate"
	// reasonIO is a file that could not be read or written.
	reasonIO = "io"
)

// commentEdit is what became of one comment entry.
type commentEdit struct {
	ID string `json:"id"`
	// Status is written, unchanged, refused or did-not-run.
	Status string `json:"status"`
	// Reason names why the edit was refused or did not run, and Detail says
	// what the reason means for this edit.
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
	// Lines are the lines the comment spans once the file's edits are applied.
	Lines *format.LineRange `json:"lines,omitempty"`
}

// commentFileResult is what became of the comment entries for one file.
type commentFileResult struct {
	File  string        `json:"file"`
	Edits []commentEdit `json:"edits"`
	// Diff is the change the file's edits make, as a unified diff.
	Diff string `json:"diff,omitempty"`
	// Check is a check scoped to Diff, run once the file is written. It is nil
	// when nothing was written.
	Check *check.Report `json:"check,omitempty"`
	// CheckError says why the check of a written change could not run.
	CheckError string `json:"check_error,omitempty"`
}

// ok reports whether every edit was written or left unchanged, and the check
// of what was written passed.
func (r commentFileResult) ok() bool {
	for _, e := range r.Edits {
		if e.Status == commentRefused || (e.Status == commentNotRun && e.Reason != reasonPreview) {
			return false
		}
	}
	if r.CheckError != "" {
		return false
	}
	return r.Check == nil || r.Check.Verdict == check.VerdictPassed
}

// applyComments applies a change-set's comment entries, one file at a time.
//
// A language's write canary runs before the first comment of that language is
// written in the run, and when it fails no entry in the language runs. A
// file's entries are applied from its last comment to its first, so an edit
// never moves the lines of a comment still to come, and each rewrite is held
// to comment.Contain. The file is read again before it is written, and a file
// that changed meanwhile is left alone. After writing, the bytes on disk are
// read back and compared with the rewrite, and a check scoped to the written
// change runs over the file.
//
// With preview set, nothing is written or checked, and each file's diff is
// reported.
func (a *App) applyComments(ctx context.Context, cmd Command, entries []changeEntry, preview bool, backupSuffix string) []commentFileResult {
	a.InitRegistries()
	byFile := map[string][]changeEntry{}
	var order []string
	for _, e := range entries {
		if _, seen := byFile[e.File]; !seen {
			order = append(order, e.File)
		}
		byFile[e.File] = append(byFile[e.File], e)
	}
	formats, formatsErr := a.newCheckFormats(cmd)
	canaries := map[string]error{}
	results := make([]commentFileResult, 0, len(order))
	for _, file := range order {
		w := &commentFileWrite{app: a, cmd: cmd, file: file, entries: byFile[file], preview: preview, backupSuffix: backupSuffix}
		w.result = commentFileResult{File: file, Edits: make([]commentEdit, len(w.entries))}
		for i, e := range w.entries {
			w.result.Edits[i] = commentEdit{ID: e.ID}
		}
		switch {
		case ctx.Err() != nil:
			w.notRun(reasonIO, ctx.Err().Error())
		case formatsErr != nil:
			w.notRun(reasonIO, formatsErr.Error())
		default:
			w.apply(ctx, formats, canaries)
		}
		results = append(results, w.result)
	}
	return results
}

// commentFileWrite applies one file's comment entries.
type commentFileWrite struct {
	app          *App
	cmd          Command
	file         string
	entries      []changeEntry
	preview      bool
	backupSuffix string
	result       commentFileResult
}

// notRun marks every edit not already refused or unchanged as did-not-run.
func (w *commentFileWrite) notRun(reason, detail string) {
	for i := range w.result.Edits {
		e := &w.result.Edits[i]
		if e.Status == "" || e.Status == commentWritten {
			e.Status, e.Reason, e.Detail, e.Lines = commentNotRun, reason, detail, nil
		}
	}
	w.result.Diff = ""
}

func (w *commentFileWrite) refuse(i int, reason, detail string) {
	e := &w.result.Edits[i]
	e.Status, e.Reason, e.Detail = commentRefused, reason, detail
}

func (w *commentFileWrite) apply(ctx context.Context, formats *checkFormats, canaries map[string]error) {
	src, err := os.ReadFile(w.file)
	if err != nil {
		w.notRun(reasonIO, err.Error())
		return
	}
	p, ok := w.app.commentProviderForEdit(w.file, formats)
	if !ok {
		for i := range w.entries {
			w.refuse(i, string(comment.RefusedUnsupported), "no comment layer reads "+DisplayName(w.file))
		}
		return
	}
	lang := p.Language()
	if _, writes := p.(comment.Rewriter); !writes {
		for i := range w.entries {
			w.refuse(i, string(comment.RefusedUnsupported), "comments in "+lang+" are read and not written")
		}
		return
	}
	if _, verified := canaries[lang]; !verified {
		canaries[lang] = comment.VerifyRewriter(p, rewriteComment)
	}
	if err := canaries[lang]; err != nil {
		w.notRun(reasonCanary, "the "+lang+" comment write canary failed, so no "+lang+" comment is written in this run: "+err.Error())
		return
	}

	directives := formats.directivesFor(w.file)
	out := src
	seen := map[string]bool{}
	for _, i := range w.bottomUp(p, src, directives) {
		e := w.entries[i]
		if seen[e.ID] {
			w.refuse(i, reasonDuplicate, "the change-set rewrites "+e.ID+" more than once; send one entry for it")
			continue
		}
		seen[e.ID] = true
		r, err := rewriteComment(p, w.file, out, directives, comment.Target{ID: e.ID, Fingerprint: e.CommentSHA256, Prose: e.CurrentText, Lines: e.Lines}, e.Text, comment.RenderOptions{Width: e.Width})
		if refusal, ok := comment.AsRefusal(err); ok {
			w.refuse(i, string(refusal.Reason), refusal.Detail)
			continue
		}
		if err != nil {
			w.result.Edits[i].Status, w.result.Edits[i].Reason, w.result.Edits[i].Detail = commentNotRun, reasonIO, err.Error()
			continue
		}
		w.result.Edits[i].Status = commentUnchanged
		if r.Changed {
			w.result.Edits[i].Status = commentWritten
			out = r.Source
		}
	}

	if bytes.Equal(out, src) {
		w.locateEdits(p, out, directives)
		return
	}
	w.result.Diff = unifiedDiff(w.file, src, out)
	if w.preview {
		w.locateEdits(p, out, directives)
		for i := range w.result.Edits {
			if e := &w.result.Edits[i]; e.Status == commentWritten {
				e.Status, e.Reason, e.Detail = commentNotRun, reasonPreview, "--diff shows the edit and writes nothing"
			}
		}
		return
	}
	if err := w.write(src, out); err != nil {
		w.notRun(err.reason, err.detail)
		return
	}
	w.locateEdits(p, out, directives)
	report, cerr := w.app.checkWrittenComments(ctx, w.cmd, w.file, w.result.Diff)
	if cerr != nil {
		w.result.CheckError = cerr.Error()
		return
	}
	w.result.Check = &report
}

// bottomUp orders the entries from the last comment in the file to the first.
// An entry naming no comment the file holds comes last, in change-set order,
// and is refused by the rewrite.
func (w *commentFileWrite) bottomUp(p comment.Provider, src []byte, directives comment.Directives) []int {
	start := map[string]int{}
	if located, err := comment.Locate(p, w.file, src, directives); err == nil {
		extents := located.Extents()
		for _, x := range extents {
			start[x.Block] = x.Start
		}
	}
	order := make([]int, len(w.entries))
	for i := range order {
		order[i] = i
	}
	at := func(i int) int {
		if s, ok := start[w.entries[i].ID]; ok {
			return s
		}
		return -1
	}
	slices.SortStableFunc(order, func(x, y int) int { return at(y) - at(x) })
	return order
}

// locateEdits records the lines each written or unchanged comment spans in the
// rewritten file.
func (w *commentFileWrite) locateEdits(p comment.Provider, out []byte, directives comment.Directives) {
	located, err := comment.Locate(p, w.file, out, directives)
	if err != nil {
		return
	}
	lines := map[string]format.LineRange{}
	for _, x := range located.Extents() {
		lines[x.Block] = x.Lines
	}
	for i := range w.result.Edits {
		e := &w.result.Edits[i]
		if l, ok := lines[e.ID]; ok && (e.Status == commentWritten || e.Status == commentUnchanged) {
			e.Lines = &l
		}
	}
}

type writeError struct{ reason, detail string }

// write replaces the file's bytes with out, once the file is found to hold src
// still. It writes a temporary file beside it and renames it into place, so a
// failed write leaves the file as it was, and reads the result back.
func (w *commentFileWrite) write(src, out []byte) *writeError {
	now, err := os.ReadFile(w.file)
	if err != nil {
		return &writeError{reasonIO, err.Error()}
	}
	if !bytes.Equal(now, src) {
		return &writeError{string(comment.RefusedStale), "the file changed while its comment edits were applied; read it again and send the edits again"}
	}
	info, err := os.Stat(w.file)
	if err != nil {
		return &writeError{reasonIO, err.Error()}
	}
	mode := info.Mode().Perm()
	if w.backupSuffix != "" {
		if err := os.WriteFile(w.file+w.backupSuffix, src, mode); err != nil {
			return &writeError{reasonIO, "write backup: " + err.Error()}
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(w.file), "."+filepath.Base(w.file)+".kapi-*")
	if err != nil {
		return &writeError{reasonIO, err.Error()}
	}
	_, werr := tmp.Write(out)
	cerr := tmp.Close()
	if err := firstErr(werr, cerr, os.Chmod(tmp.Name(), mode)); err != nil {
		_ = os.Remove(tmp.Name())
		return &writeError{reasonIO, err.Error()}
	}
	if err := os.Rename(tmp.Name(), w.file); err != nil {
		_ = os.Remove(tmp.Name())
		return &writeError{reasonIO, err.Error()}
	}
	written, err := os.ReadFile(w.file)
	if err != nil {
		return &writeError{reasonIO, err.Error()}
	}
	if !bytes.Equal(written, out) {
		return &writeError{reasonIO, "the bytes read back from the file differ from the rewrite"}
	}
	return nil
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// commentProviderForEdit returns the provider that reads the comments of the
// file an edit names: the one its declared or detected format supplies, else
// the one for its language.
func (a *App) commentProviderForEdit(file string, formats *checkFormats) (comment.Provider, bool) {
	name, _ := formats.forFile(a, file)
	if name == "" {
		if id, err := a.FormatReg.Detect(file, registry.DetectOptions{ExtensionOnly: true}); err == nil {
			name = string(id)
		}
	}
	if name != "" {
		if p, ok := commentProviders.ForFormat(name); ok {
			return p, true
		}
	}
	return a.commentProviderFor(file)
}

// unifiedDiff is the change from before to after in file, as git writes a
// unified diff of it, with paths relative to the file's directory.
func unifiedDiff(file string, before, after []byte) string {
	name := filepath.ToSlash(filepath.Base(file))
	text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        diffLines(before),
		B:        diffLines(after),
		FromFile: "a/" + name,
		ToFile:   "b/" + name,
		Context:  3,
	})
	if err != nil {
		return ""
	}
	return text
}

// diffLines splits content after each line feed, ending the last line with one
// when the file does not.
func diffLines(content []byte) []string {
	lines := strings.SplitAfter(string(content), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
		lines[n-1] += "\n"
	}
	return lines
}

// checkWrittenComments checks what a written comment edit changed: the diff,
// scoped the way `kapi check --diff-file` scopes one, under the governance at
// the file's point.
func (a *App) checkWrittenComments(ctx context.Context, cmd Command, file, diff string) (check.Report, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return check.Report{}, err
	}
	files, err := diffscope.Parse([]byte(diff))
	if err != nil {
		return check.Report{}, fmt.Errorf("parse the diff of %s: %w", DisplayName(file), err)
	}
	execution := newCheckExecution()
	voice, err := a.newCheckVoice(cmd, execution.warningSink())
	if err != nil {
		return check.Report{}, err
	}
	defer voice.close()
	vocab, err := a.newCheckTerms(cmd)
	if err != nil {
		return check.Report{}, err
	}
	formats, err := a.newCheckFormats(cmd)
	if err != nil {
		return check.Report{}, err
	}
	src := &diffSource{label: "comment edit", root: filepath.Dir(abs), files: files}
	return a.runDiffCheck(ctx, diffCheckRun{
		src:   src,
		cmd:   cmd,
		opts:  checkRunOptions{formats: formats, execution: execution},
		voice: voice,
		vocab: vocab,
		gate:  check.DefaultGate(),
	})
}

// printCommentResults writes a short human summary of each file's comment
// edits and the check of what was written.
func printCommentResults(w io.Writer, results []commentFileResult) {
	for _, f := range results {
		for _, e := range f.Edits {
			fmt.Fprintf(w, "comment %s %s: %s", DisplayName(f.File), e.ID, e.Status)
			switch {
			case e.Reason != "":
				fmt.Fprintf(w, " (%s: %s)", e.Reason, e.Detail)
			case e.Lines != nil:
				fmt.Fprintf(w, " (lines %d-%d)", e.Lines.First, e.Lines.Last)
			}
			fmt.Fprintln(w)
		}
		switch {
		case f.CheckError != "":
			fmt.Fprintf(w, "check %s: did not run (%s)\n", DisplayName(f.File), f.CheckError)
		case f.Check != nil:
			fmt.Fprintf(w, "check %s: %s, %d finding(s)\n", DisplayName(f.File), f.Check.Verdict, len(f.Check.Findings))
			for _, d := range f.Check.Findings {
				fmt.Fprintf(w, "  %s %s %s: %s\n", d.Severity, d.Rule, d.Location.Block, d.Message)
			}
		}
	}
}
