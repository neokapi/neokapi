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

	"github.com/neokapi/neokapi/core/change"
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

// commentDoc reports whether path is a source file whose comments are what
// kapi edits in it: no format reads the file, and a comment reader reads its
// language, or would once the plugin that reads it is installed.
func (a *App) commentDoc(path string) bool {
	if _, err := a.FormatReg.Detect(path, registry.DetectOptions{ExtensionOnly: true}); err == nil {
		return false
	}
	if _, ok := a.commentProviderFor(path); ok {
		return true
	}
	_, plugin := commentPluginHintFor(path)
	return plugin
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
// address both comments and documents is refused.
func (a *App) commentSet(set change.Set, root string) (bool, error) {
	a.InitRegistries()
	comments, docs := 0, 0
	for _, op := range set.Ops {
		if at := op.At.Doc; at != "" && a.commentDoc(docPath(root, at)) {
			comments++
			continue
		}
		docs++
	}
	if comments > 0 && docs > 0 {
		return false, errors.New("apply: this change set edits code comments and documents; " +
			"a comment is rewritten through its language's comment layer and a document through its format, so send each in a change set of its own")
	}
	return comments > 0, nil
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
// refused by the rewrite; otherwise it previews or writes every comment.
func (a *App) applyCommentSet(ctx context.Context, cmd Command, set change.Set, root, backup string, trust *formatterTrust) (*change.Result, error) {
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
		return nil, err
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
		return finishCommentSet(res, refused), nil
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
				return nil, ioErr
			}
			refuse(o.i, cerr)
		}
	}
	if refused >= 0 {
		return finishCommentSet(res, refused), nil
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
		return res, nil
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
		return finishCommentSet(res, refused), nil
	default:
		res.Status = change.SetApplied
	}
	return res, nil
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
	case reasonDuplicate, string(comment.RefusedText), string(comment.RefusedTerminator):
		return &change.Error{Code: change.CodeInvalid, Field: "text", Message: msg}, nil
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
