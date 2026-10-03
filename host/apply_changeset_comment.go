package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/registry"
)

// A code comment is a block of a source file: kapi check and kapi inspect
// report it under the id core/comment gives it (func/Parse), and a change set
// rewrites it with set_content. Its revision is "r:" and the first sixteen hex
// digits of the comment's fingerprint (the comment_sha256 kapi check
// reports), so an edit to a comment whose bytes changed since it was read is
// refused stale. The comment write path keeps its guarantees: the file is read
// again before it is written, every byte outside the comment stays, the result
// must parse, the language's formatter must agree, and what was written is
// checked again.
//
// A change set edits code comments or documents. The comment path writes
// through core/comment rather than a format's writer, so one change set holds
// operations of one kind, and a set that mixes them is refused before
// anything is written.

// commentRevision is the revision of a comment whose fingerprint is fp.
func commentRevision(fp string) string {
	if len(fp) < 16 {
		return ""
	}
	return "r:" + fp[:16]
}

// commentDocs tells which files are source files whose comments are what kapi
// edits in them: a comment reader reads the file's language, or would once
// the plugin that reads it is installed, and no format reads the file. A
// format reads it when --format names one, when the project binds the file to
// one, or when detection finds one by the file's extension or its content, as
// it finds a Qt Linguist catalog in a .ts file, an extension TypeScript
// shares.
type commentDocs struct {
	app  *App
	root string
	// index is the recipe's content, resolved on first use; nil outside a
	// project.
	index func() *projectChangeIndex
}

// newCommentDocs tells comment documents for the project at recipe, "" none.
func (a *App) newCommentDocs(recipe string) *commentDocs {
	c := &commentDocs{app: a}
	if recipe != "" {
		c.root = filepath.Dir(recipe)
		c.index = a.projectIndex(recipe)
	}
	return c
}

// is reports whether the file at path is a comment document.
func (c *commentDocs) is(path string) bool {
	a := c.app
	if a.FormatFlag != "" || path == StdinName {
		return false
	}
	if _, member := parseEntryLocator(path); member {
		return false
	}
	if _, ok := a.commentProviderFor(path); !ok {
		if _, plugin := commentPluginHintFor(path); !plugin {
			return false
		}
	}
	return !a.formatReads(path) && !c.bound(path)
}

// bound reports whether the project binds the file at path to a format: a
// source the recipe claims for its content, or the file of one of its
// translations. A file declared for its comments alone is not bound.
func (c *commentDocs) bound(path string) bool {
	if c.index == nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(c.root, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	ix := c.index()
	if ix == nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	_, source := ix.sources[rel]
	_, target := ix.byTarget[rel]
	return source || target
}

// formatReads reports whether detection finds a format for the file at path,
// by its extension or, when the file can be opened, by its content.
func (a *App) formatReads(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		_, derr := a.FormatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true})
		return derr == nil
	}
	defer f.Close()
	_, ok := a.explicitOrDetected(path, f)
	return ok
}

// docPath is the file a document reference names, under root.
func docPath(root, doc string) string {
	p := filepath.FromSlash(doc)
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(root, p)
}

// commentSet reports whether set edits code comments. A set whose operations
// address both comments and documents is refused, and so is one that edits
// comments and writes to the project's stores (a term, a content-memory pair,
// the recipe).
func (a *App) commentSet(set change.Set, root string, comments *commentDocs) (bool, error) {
	a.InitRegistries()
	var onComments, onDocs, other []change.Kind
	for _, op := range set.Ops {
		switch at := op.At.Doc; {
		case at == "":
			other = append(other, op.Kind)
		case comments.is(docPath(root, at)):
			onComments = append(onComments, op.Kind)
		default:
			onDocs = append(onDocs, op.Kind)
		}
	}
	switch {
	case len(onComments) == 0:
		return false, nil
	case len(onDocs) > 0:
		return false, errors.New("apply: this change set edits code comments and documents; " +
			"a comment is rewritten through its language's comment layer and a document through its format, so send each in a change set of its own")
	case len(other) > 0:
		return false, fmt.Errorf("apply: this change set edits code comments and also holds a %s operation; "+
			"a change set that rewrites comments holds only their set_content operations, so send the %s operation in a change set of its own", other[0], other[0])
	}
	return true, nil
}

// locatedComment is a comment as a read of its file found it.
type locatedComment struct {
	fingerprint string
	prose       string
}

// commentFile is a source file's comments as one read found them, by id, and
// the digest of the bytes read.
type commentFile struct {
	comments map[string]locatedComment
	digest   string
}

// commentsIn locates the comments of the file at path. A file no comment
// reader reads, or whose comments could not be located, holds none here; the
// rewrite reports why.
func (a *App) commentsIn(path string, formats *checkFormats) (commentFile, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return commentFile{}, err
	}
	sum := sha256.Sum256(src)
	out := commentFile{comments: map[string]locatedComment{}, digest: "sha256:" + hex.EncodeToString(sum[:])}
	p, ok := a.commentProviderForEdit(path, formats)
	if !ok {
		return out, nil
	}
	located, err := comment.Locate(p, path, src, formats.directivesFor(path))
	if err != nil {
		return out, nil
	}
	r, _ := p.(comment.Rewriter)
	for i, x := range located.Extents() {
		c := located.Comments[i]
		lc := locatedComment{fingerprint: comment.Fingerprint(src, c)}
		if r != nil {
			if prose, perr := r.Prose(src, c); perr == nil {
				lc.prose = prose
			}
		}
		out.comments[x.Block] = lc
	}
	return out, nil
}

// commentOp is one operation of a comment change set, as the comment path
// applies it.
type commentOp struct {
	i     int
	path  string
	entry changeEntry
}

// applyCommentSet applies a change set whose every operation rewrites a code
// comment. It refuses the whole set, writing nothing, when an operation is
// malformed, names a comment that changed since it was read, or would be
// refused by the rewrite; otherwise it previews or writes every comment. It
// returns the result, and what the comment path's last run did in each file:
// the preview's outcomes when the set was refused or previewed, the write's
// when it ran, none when the set was refused before the comment path ran.
func (a *App) applyCommentSet(ctx context.Context, cmd Command, set change.Set, root, backup string, trust *formatterTrust) (*change.Result, []commentFileResult, error) {
	res := &change.Result{Schema: change.ResultSchemaID, Ops: make([]change.OpResult, len(set.Ops)), Docs: []change.DocResult{}}
	for i, op := range set.Ops {
		at := op.At
		res.Ops[i] = change.OpResult{I: i, Op: op.Kind, At: &at}
	}
	refused := -1
	refuse := func(i int, e *change.Error) {
		res.Ops[i].Status, res.Ops[i].Error = change.OpRefused, e
		if refused < 0 {
			refused = i
		}
	}

	formats, err := a.newCheckFormats(cmd)
	if err != nil {
		return nil, nil, err
	}
	var ops []commentOp
	seen := map[string]int{}
	located := map[string]commentFile{}
	for i, op := range set.Ops {
		body, ok := op.Body.(*change.SetContent)
		switch {
		case op.Kind != change.KindSetContent || !ok:
			refuse(i, &change.Error{Code: change.CodeUnsupported, Capability: "comment." + string(op.Kind),
				Message: "a code comment is rewritten whole with set_content and its new prose in text"})
			continue
		case !op.At.Edition.IsZero():
			refuse(i, &change.Error{Code: change.CodeUnsupported, Capability: "edition", Field: "at/edition",
				Message: "a code comment has one edition, the one the file is written in"})
			continue
		case body.Text == nil || len(body.Path) > 0:
			refuse(i, &change.Error{Code: change.CodeUnsupported, Capability: "comment.runs", Field: "text",
				Message: "a code comment takes its new prose in text, without comment markers"})
			continue
		case op.IfMatch == "absent":
			refuse(i, &change.Error{Code: change.CodeUnsupported, Capability: "comment.create", Field: "if_match",
				Message: "an edit rewrites a comment the file holds; it never adds one"})
			continue
		}
		path := docPath(root, op.At.Doc)
		key := path + "\x00" + op.At.Block
		if j, dup := seen[key]; dup {
			refuse(i, &change.Error{Code: change.CodeInvalid, Field: "at/block",
				Message: fmt.Sprintf("operation %d rewrites %s too; send one operation per comment", j, op.At.Block)})
			continue
		}
		seen[key] = i
		if _, ok := located[path]; !ok {
			file, lerr := a.commentsIn(path, formats)
			if lerr != nil {
				refuse(i, &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + op.At.Doc + ": " + lerr.Error()})
				continue
			}
			located[path] = file
		}
		e := changeEntry{Kind: kindComment, File: path, ID: op.At.Block, Text: *body.Text}
		if c, ok := located[path].comments[op.At.Block]; ok {
			rev := commentRevision(c.fingerprint)
			if op.IfMatch != change.AnyRevision && op.IfMatch != rev {
				refuse(i, &change.Error{Code: change.CodeStale, Field: "if_match",
					Message: fmt.Sprintf("comment %s of %s is at %s, not %s", op.At.Block, op.At.Doc, rev, op.IfMatch)})
				res.Ops[i].Current = &change.Current{Rev: rev, Text: c.prose}
				continue
			}
			e.CommentSHA256 = c.fingerprint
			res.Ops[i].Before = rev
		}
		ops = append(ops, commentOp{i: i, path: path, entry: e})
	}
	if refused >= 0 {
		return finishCommentSet(res, refused), nil, nil
	}

	entries := make([]changeEntry, len(ops))
	for j, o := range ops {
		entries[j] = o.entry
	}
	// The preview runs every rewrite and writes nothing, so a refusal in any
	// file refuses the whole change set before one comment is written.
	preview := a.applyComments(ctx, cmd, entries, true, "", trust, "")
	byOp := commentOutcomes(ops, preview)
	for _, o := range ops {
		if e := byOp[o.i]; e != nil && (e.Status == commentRefused || (e.Status == commentNotRun && e.Reason != reasonPreview)) {
			cerr, ioErr := commentError(*e)
			if ioErr != nil {
				return nil, preview, ioErr
			}
			refuse(o.i, cerr)
		}
	}
	if refused >= 0 {
		return finishCommentSet(res, refused), preview, nil
	}
	if set.Mode == change.ModePreview {
		for _, o := range ops {
			res.Ops[o.i].Status = change.OpPreviewed
			if e := byOp[o.i]; e != nil && e.Status == commentUnchanged {
				res.Ops[o.i].Status = change.OpUnchanged
			}
		}
		for _, f := range preview {
			res.Docs = append(res.Docs, commentDocResult(root, f, located[f.File].digest, false))
		}
		res.Status = change.SetPreviewed
		return res, preview, nil
	}

	results := a.applyComments(ctx, cmd, entries, false, backup, trust, "")
	byOp = commentOutcomes(ops, results)
	written := false
	for _, f := range results {
		d := commentDocResult(root, f, located[f.File].digest, true)
		written = written || d.Written
		res.Docs = append(res.Docs, d)
	}
	after := map[string]commentFile{}
	for _, o := range ops {
		e := byOp[o.i]
		switch {
		case e == nil:
			refuse(o.i, &change.Error{Code: change.CodeInvalid, Message: "the comment path gave no outcome for this operation"})
		case e.Status == commentWritten:
			res.Ops[o.i].Status = change.OpApplied
			if _, ok := after[o.path]; !ok {
				after[o.path], _ = a.commentsIn(o.path, formats)
			}
			if c, ok := after[o.path].comments[o.entry.ID]; ok {
				res.Ops[o.i].After = commentRevision(c.fingerprint)
			}
		case e.Status == commentUnchanged:
			res.Ops[o.i].Status = change.OpUnchanged
			res.Ops[o.i].After = res.Ops[o.i].Before
		default:
			cerr, ioErr := commentError(*e)
			if ioErr != nil {
				cerr = &change.Error{Code: change.CodeDocChanged, Message: ioErr.Error()}
			}
			refuse(o.i, cerr)
		}
	}
	switch {
	case refused >= 0 && written:
		res.Status = change.SetPartial
	case refused >= 0:
		return finishCommentSet(res, refused), results, nil
	default:
		res.Status = change.SetApplied
	}
	return res, results, nil
}

// applyCommentChange is kapi apply's comment branch: it applies a comment
// change set under the execution trust the command grants the project's
// formatter, asking at a terminal unless the change set came from standard
// input, which leaves no one to answer.
func (a *App) applyCommentChange(cmd Command, set change.Set, root, backup string, fromStdin bool) (*change.Result, []commentFileResult, error) {
	trust := a.applyFormatterTrust(cmd, fromStdin)
	return a.applyCommentSet(cmd.Context(), cmd, set, root, backup, trust)
}

// commentSetExit is the exit kapi apply returns for a comment change set's
// result. A written comment is checked once it is written, scoped to the
// change, and under the enforcing gate a failing finding there, or a check
// that could not run, sends the caller back to fix the prose, as a refusal
// does. Any other result exits as a change set does.
func commentSetExit(set change.Set, res *change.Result) error {
	if set.Gate != change.GateReport && failsAfterWrite(res) {
		return WithExitCode(ExitGate, ErrSilentExit)
	}
	return changeResultExit(res)
}

// failsAfterWrite reports whether the check of a written document found a
// failing finding.
func failsAfterWrite(res *change.Result) bool {
	for _, d := range res.Docs {
		if d.Written && slices.ContainsFunc(d.Findings, func(f change.Finding) bool { return f.Fails }) {
			return true
		}
	}
	return false
}

// finishCommentSet marks every operation not refused as held back by the
// first refusal, and the set refused.
func finishCommentSet(res *change.Result, refused int) *change.Result {
	for i := range res.Ops {
		if res.Ops[i].Status == change.OpRefused {
			continue
		}
		blocked := refused
		res.Ops[i].Status, res.Ops[i].BlockedBy = change.OpNotApplied, &blocked
		res.Ops[i].Before = ""
	}
	res.Status = change.SetRefused
	return res
}

// commentOutcomes pairs each operation with what became of its entry. The
// comment path groups entries by file in the order they came and keeps each
// file's edits in entry order.
func commentOutcomes(ops []commentOp, results []commentFileResult) map[int]*commentEdit {
	next := map[string]int{}
	byFile := map[string]*commentFileResult{}
	for i := range results {
		byFile[results[i].File] = &results[i]
	}
	out := map[int]*commentEdit{}
	for _, o := range ops {
		f := byFile[o.path]
		if f == nil {
			continue
		}
		j := next[o.path]
		next[o.path]++
		if j < len(f.Edits) {
			out[o.i] = &f.Edits[j]
		}
	}
	return out
}

// commentDocResult is the outcome for one file of a comment change set, whose
// bytes had the digest before when it was read.
func commentDocResult(root string, f commentFileResult, before string, apply bool) change.DocResult {
	d := change.DocResult{Doc: relRef(root, f.File), Home: "file", Before: before}
	if apply && slices.ContainsFunc(f.Edits, func(e commentEdit) bool { return e.Status == commentWritten }) {
		d.Written = true
		if data, err := os.ReadFile(f.File); err == nil {
			sum := sha256.Sum256(data)
			after := "sha256:" + hex.EncodeToString(sum[:])
			d.After = &after
		}
		if f.Check != nil {
			for _, x := range f.Check.Findings {
				d.Findings = append(d.Findings, change.Finding{Rule: x.Rule, Message: x.Message, Fails: x.Fails, Suggested: x.Suggested})
			}
			// A check that read nothing is never a pass.
			if f.Check.Verdict == check.VerdictDidNotRun {
				why := "the check of the written change read nothing"
				if len(f.Check.DidNotRun) > 0 {
					why += ": " + strings.Join(f.Check.DidNotRun, "; ")
				}
				d.Findings = append(d.Findings, change.Finding{Rule: "check.did-not-run", Message: why, Fails: true})
			}
		}
	}
	if f.CheckError != "" {
		d.Findings = append(d.Findings, change.Finding{Rule: "check.did-not-run", Message: f.CheckError, Fails: true})
	}
	if !apply {
		d.Diff = f.Diff
	}
	return d
}

// relRef is the reference of the file at path under root: a relative,
// slash-separated path, or the path itself when it lies outside root.
func relRef(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// commentError is the refusal a comment edit the comment path refused or did
// not run maps to. A file that could not be read or written is an error of
// its own, returned second.
func commentError(e commentEdit) (*change.Error, error) {
	msg := e.Detail
	if msg == "" {
		msg = e.Reason
	}
	switch e.Reason {
	case reasonIO:
		return nil, errors.New(msg)
	case string(comment.RefusedUnknown):
		return &change.Error{Code: change.CodeNotFound, Field: "at/block", Message: msg}, nil
	case string(comment.RefusedChanged), string(comment.RefusedStale):
		return &change.Error{Code: change.CodeStale, Field: "if_match", Message: msg}, nil
	case reasonDuplicate:
		return &change.Error{Code: change.CodeInvalid, Field: "at/block", Message: msg}, nil
	case string(comment.RefusedText), string(comment.RefusedTerminator):
		// Text the comment cannot hold, or that would end it early: the
		// content needs fixing, as for a guard.
		return &change.Error{Code: change.CodeGuard, Field: "text", Message: msg}, nil
	case string(comment.RefusedStructure):
		return &change.Error{Code: change.CodeGuard, Subcode: change.SubcodeStructureLost, Field: "text", Message: msg}, nil
	case string(comment.RefusedLayout), string(comment.RefusedDeprecation), string(comment.RefusedAttachment),
		string(comment.RefusedContainment), string(comment.RefusedParse):
		return &change.Error{Code: change.CodeGuard, Field: "text", Message: msg}, nil
	case string(comment.RefusedFormatter):
		if e.Status == commentRefused {
			return &change.Error{Code: change.CodeGuard, Field: "text", Message: msg}, nil
		}
		return &change.Error{Code: change.CodeUnsupported, Capability: "formatter", Message: msg}, nil
	}
	return &change.Error{Code: change.CodeUnsupported, Capability: "comment." + e.Reason, Message: msg}, nil
}
