package host

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/pmezard/go-difflib/difflib"
)

// changeKind discriminates a change-set entry. `apply` is the single write verb:
// every deliberate, reviewed change Claude proposes — a content edit or an asset
// edit (term, content memory pair, voice rule, recipe field) — is one typed entry,
// so "is this change reviewed?" has one answer for everything and the backing
// stores are written by exactly one code path.
type changeKind string

const (
	kindContent changeKind = "content"
	kindTerm    changeKind = "term"
	kindMemory  changeKind = "memory"
	kindVoice   changeKind = "voice"
	kindRecipe  changeKind = "recipe"
	kindReview  changeKind = "review"
	kindComment changeKind = "comment"
)

// changeEntry is one line of a `kapi apply` change-set (JSONL; one entry per
// line). Only the fields relevant to its Kind are populated. Content edits carry
// the block address (file + id + content_hash) and the new placeholder-rendered
// text; asset edits carry an op and the per-asset fields. Comment edits carry
// the file, the comment's id, the fingerprint or prose it was read with and
// its new prose.
type changeEntry struct {
	Kind changeKind `json:"kind" jsonschema:"change kind; use content for document wording and comment for a code comment"`

	// content and comment
	File        string `json:"file,omitempty"`
	ID          string `json:"id,omitempty" jsonschema:"the block id; for kind=comment the comment's id as check_file reports it, such as func/Parse"`
	ContentHash string `json:"content_hash,omitempty"`
	Text        string `json:"text,omitempty" jsonschema:"the new wording: for kind=content the block text with the inline placeholders extract_content shows; for kind=comment the comment's prose without comment markers"`

	// comment
	CommentSHA256 string            `json:"comment_sha256,omitempty" jsonschema:"for kind=comment: the comment_sha256 check_file reports for the comment; an edit to a comment whose bytes differ is refused as changed"`
	CurrentText   *string           `json:"current_text,omitempty" jsonschema:"for kind=comment: the comment's prose as you read it, without comment markers; guards the edit when comment_sha256 is not given"`
	Lines         *format.LineRange `json:"lines,omitempty" jsonschema:"for kind=comment: the lines check_file reported for the comment"`
	Width         int               `json:"width,omitempty" jsonschema:"for kind=comment: the column a line of prose wraps at; 0 keeps the width of the comment being rewritten, and never less than 80. The languages a comment plugin reads keep the comment's own line breaks, so width does not apply to them"`

	// asset common
	Op string `json:"op,omitempty"`

	// term
	Term     string `json:"term,omitempty"`
	Locale   string `json:"locale,omitempty"`
	Status   string `json:"status,omitempty"`
	Replaces string `json:"replaces,omitempty"`
	// DoNotTranslate, for kind=term, sets (true) or clears (false) the
	// do-not-translate flag on the term's concept; omitted leaves it.
	DoNotTranslate *bool `json:"do_not_translate,omitempty" jsonschema:"for kind=term: true keeps the term verbatim in every language, false clears that, omitted leaves it"`

	// tm
	Source       string `json:"source,omitempty"`
	Target       string `json:"target,omitempty"`
	SourceLocale string `json:"source_locale,omitempty"`
	TargetLocale string `json:"target_locale,omitempty"`

	// brand
	List        string `json:"list,omitempty"`
	Replacement string `json:"replacement,omitempty" jsonschema:"replacement term for kind=voice; content entries use text"`
	Severity    string `json:"severity,omitempty"`

	// recipe
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`

	// Actor is who wrote this entry, recorded on the context operation the
	// entry produces (core/contextop). Omitted reads as a person, which is what
	// someone running `kapi apply` is. An agent naming itself here is refused
	// for an asset entry, because applying one is a decision and only a person
	// makes those; an agent proposes instead.
	Actor *contextop.Actor `json:"actor,omitempty" jsonschema:"who is making this change; omit unless you are an agent recording on someone's behalf"`
	// Evidence is where the wording behind an asset entry was seen, recorded on
	// the operation so the decision can be argued with later.
	Evidence []contextop.Evidence `json:"evidence,omitempty" jsonschema:"for asset entries: where the wording behind this decision was seen"`
}

// assetResult is the outcome of one asset entry, surfaced in the ApplyReport.
type assetResult struct {
	Kind   changeKind `json:"kind"`
	Op     string     `json:"op,omitempty"`
	Target string     `json:"target,omitempty"`
	Status string     `json:"status"` // applied | skipped | error
	Detail string     `json:"detail,omitempty"`
}

// applyOutput is the JSON-first report of an apply pass. Content outcomes are
// bucketed by block (applied/skipped/stale/guard_failed); asset outcomes list
// one result per entry. stale or guard_failed content, or an asset error, means
// the change-set did not fully land and the command exits non-zero so a fix
// loop re-inspects and retries.
type applyOutput struct {
	Content struct {
		Applied     []string `json:"applied,omitempty"`
		Skipped     []string `json:"skipped,omitempty"`
		Stale       []string `json:"stale,omitempty"`
		GuardFailed []string `json:"guard_failed,omitempty"`
	} `json:"content"`
	Assets []assetResult `json:"assets,omitempty"`
	// Comments holds each file's comment edits, the diff they made and the
	// check of it. A refused edit, an edit that did not run, or a check that
	// did not pass means the change-set did not fully land.
	Comments []commentFileResult `json:"comments,omitempty"`
}

func (o *applyOutput) ok() bool {
	for _, c := range o.Comments {
		if !c.ok() {
			return false
		}
	}
	return len(o.Content.Stale) == 0 && len(o.Content.GuardFailed) == 0 && !o.assetErr()
}

func (o *applyOutput) assetErr() bool {
	for _, a := range o.Assets {
		if a.Status == "error" {
			return true
		}
	}
	return false
}

func (a *App) RunApply(cmd Command, path string, diff bool, backupSuffix string, asJSON bool) error {
	ctx := cmd.Context()
	entries, err := readChangeSet(ctx, path)
	if err != nil {
		return err
	}
	if err := validateContentWording(entries); err != nil {
		return err
	}

	var out applyOutput

	// Content entries grouped by file → one faithful round-trip per file.
	byFile := map[string][]changeEntry{}
	var fileOrder []string
	var comments []changeEntry
	for _, e := range entries {
		switch e.Kind {
		case kindContent:
			if e.File == "" {
				return fmt.Errorf("apply: content entry for block %q has no \"file\"", e.ID)
			}
			if _, seen := byFile[e.File]; !seen {
				fileOrder = append(fileOrder, e.File)
			}
			byFile[e.File] = append(byFile[e.File], e)
		case kindComment:
			comments = append(comments, e)
		case kindTerm, kindMemory, kindVoice, kindRecipe:
			res := a.applyRecordedAssetEntry(ctx, cmd, e)
			out.Assets = append(out.Assets, res)
		case kindReview:
			res := a.applyReviewEntry(ctx, cmd, e)
			out.Assets = append(out.Assets, res)
		case "":
			return errors.New("apply: change-set entry has no \"kind\"")
		default:
			return fmt.Errorf("apply: unknown change kind %q", e.Kind)
		}
	}

	// --diff writes a unified diff for a person and --json a report for a
	// program. With both, the diff goes to stderr, so stdout holds one JSON
	// document; the report carries each comment's diff as well.
	diffOut := cmd.OutOrStdout()
	if asJSON {
		diffOut = cmd.ErrOrStderr()
	}

	for _, file := range fileOrder {
		report := &coretools.ApplyReport{}
		byID, byHash := buildEditMaps(byFile[file])
		t := coretools.NewApplyEditsTool(byID, byHash, report)
		if diff {
			if _, derr := a.rewriteDiffFile(ctx, file, t, diffOut); derr != nil {
				if errors.Is(derr, context.Canceled) {
					return derr
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "apply: %s: %v\n", DisplayName(file), derr)
			}
		} else {
			if derr := a.EditDocument(ctx, file, t, "", true, backupSuffix, cmd.OutOrStdout()); derr != nil {
				if errors.Is(derr, context.Canceled) {
					return derr
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "apply: %s: %v\n", DisplayName(file), derr)
			}
		}
		out.Content.Applied = append(out.Content.Applied, report.Applied...)
		out.Content.Skipped = append(out.Content.Skipped, report.Skipped...)
		out.Content.Stale = append(out.Content.Stale, report.Stale...)
		out.Content.GuardFailed = append(out.Content.GuardFailed, report.GuardFailed...)
	}
	if len(comments) > 0 {
		out.Comments = a.applyComments(ctx, cmd, comments, diff, backupSuffix, a.applyFormatterTrust(cmd, path == "" || path == StdinName), "")
		if diff {
			for _, f := range out.Comments {
				fmt.Fprint(diffOut, f.Diff)
			}
			if !asJSON {
				printCommentResults(cmd.ErrOrStderr(), out.Comments)
			}
		}
	}

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return err
		}
	} else if !diff {
		printApplyReport(cmd.ErrOrStderr(), &out)
	}

	if !out.ok() {
		// A drift / guard miss or asset error means work remains: exit on the
		// gate code so a fix loop re-inspects and retries, distinct from an
		// operational failure.
		return WithExitCode(ExitGate, ErrSilentExit)
	}
	return nil
}

// Validate wording fields before either surface starts applying the change-set.
func validateContentWording(entries []changeEntry) error {
	for i, e := range entries {
		if e.Kind == kindContent && e.Replacement != "" {
			return fmt.Errorf("content entry %d for block %q: put the new wording in \"text\"; \"replacement\" belongs to voice rules", i+1, e.ID)
		}
		if e.Kind != kindComment {
			continue
		}
		switch {
		case e.File == "":
			return fmt.Errorf("comment entry %d for %q has no \"file\"", i+1, e.ID)
		case e.ID == "":
			return fmt.Errorf("comment entry %d in %s has no \"id\"; use the comment's id as kapi check reports it", i+1, e.File)
		case e.Replacement != "":
			return fmt.Errorf("comment entry %d for %q: put the new prose in \"text\"; \"replacement\" belongs to voice rules", i+1, e.ID)
		case e.ContentHash != "":
			return fmt.Errorf("comment entry %d for %q: a comment is guarded by the \"comment_sha256\" kapi check reports, not by \"content_hash\"", i+1, e.ID)
		case e.CommentSHA256 == "" && e.CurrentText == nil:
			return fmt.Errorf("comment entry %d for %q has no guard: pass the \"comment_sha256\" kapi check reports for the comment, or its prose as you read it in \"current_text\"", i+1, e.ID)
		}
	}
	return nil
}

// buildEditMaps splits content entries into an ID-keyed and a hash-keyed lookup
// for the apply-edits tool: entries with an ID resolve by ID, ID-less entries
// resolve by content_hash.
func buildEditMaps(entries []changeEntry) (byID, byHash map[string]coretools.Edit) {
	byID = map[string]coretools.Edit{}
	byHash = map[string]coretools.Edit{}
	for _, e := range entries {
		edit := coretools.Edit{Text: e.Text, ContentHash: e.ContentHash}
		if e.ID != "" {
			byID[e.ID] = edit
		} else if e.ContentHash != "" {
			byHash[e.ContentHash] = edit
		}
	}
	return byID, byHash
}

// readChangeSet reads a JSONL change-set from path (or stdin when path is empty
// or "-"). A leading "[" is also accepted as a JSON array, for convenience.
func readChangeSet(ctx context.Context, path string) ([]changeEntry, error) {
	var r io.Reader
	if path == "" || path == StdinName {
		r = os.Stdin
	} else {
		data, err := readContent(ctx, path)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(data)
	}

	br := bufio.NewReader(r)
	// Peek for a JSON array form.
	for {
		b, err := br.Peek(1)
		if err != nil {
			if err == io.EOF {
				return nil, nil
			}
			return nil, err
		}
		if b[0] == ' ' || b[0] == '\t' || b[0] == '\n' || b[0] == '\r' {
			_, _ = br.ReadByte()
			continue
		}
		if b[0] == '[' {
			var arr []changeEntry
			if err := json.NewDecoder(br).Decode(&arr); err != nil {
				return nil, fmt.Errorf("apply: parse change-set array: %w", err)
			}
			return arr, nil
		}
		break
	}

	// A stream of JSON values, decoded one at a time. JSONL is the shape this
	// documents and the shape `kapi apply` prints in its own examples, but a
	// decoder accepts it without caring where the newlines fall — which is what
	// lets the product's two verbs compose: `kapi status --review --json --jq
	// '…'` emits one indented object per selected unit, and that is a change-set
	// too. A line scanner rejected it on the first line of the first object.
	var entries []changeEntry
	dec := json.NewDecoder(br)
	n := 0
	for {
		var e changeEntry
		if err := dec.Decode(&e); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("apply: parse change-set entry %d: %w", n+1, err)
		}
		n++
		entries = append(entries, e)
	}
	return entries, nil
}

// rewriteDiffFile prints the per-block unified diff for one file and returns the
// number of changed blocks. The block source is rewritten in memory only (the
// applier's plan is applied to the streamed block); nothing is written to disk.
// It backs `kapi apply --diff`.
func (a *App) rewriteDiffFile(ctx context.Context, file string, t *tool.BaseTool, out io.Writer) (int, error) {
	changed := 0
	label := DisplayName(file)
	_, err := a.StreamBlocks(ctx, file, func(index int, b *model.Block) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		before := model.RunsText(b.Source)
		part := &model.Part{Type: model.PartBlock, Resource: b}
		if _, aerr := t.ApplyContext(ctx, part); aerr != nil {
			return aerr
		}
		after := model.RunsText(b.Source)
		if before == after {
			return nil
		}
		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(before),
			B:        difflib.SplitLines(after),
			FromFile: fmt.Sprintf("%s:%d (before)", label, index),
			ToFile:   fmt.Sprintf("%s:%d (after)", label, index),
			Context:  3,
		}
		text, derr := difflib.GetUnifiedDiffString(diff)
		if derr != nil {
			return derr
		}
		if _, werr := out.Write([]byte(text)); werr != nil {
			return werr
		}
		changed++
		return nil
	})
	return changed, err
}

// printApplyReport writes a short human summary of the apply outcome.
func printApplyReport(w io.Writer, out *applyOutput) {
	c := out.Content
	if n := len(c.Applied) + len(c.Skipped) + len(c.Stale) + len(c.GuardFailed); n > 0 {
		fmt.Fprintf(w, "content: %d applied, %d unchanged", len(c.Applied), len(c.Skipped))
		if len(c.Stale) > 0 {
			fmt.Fprintf(w, ", %d stale (source drifted, re-inspect)", len(c.Stale))
		}
		if len(c.GuardFailed) > 0 {
			fmt.Fprintf(w, ", %d rejected (would corrupt inline codes)", len(c.GuardFailed))
		}
		fmt.Fprintln(w)
	}
	for _, ar := range out.Assets {
		target := ar.Target
		if target == "" {
			target = string(ar.Kind)
		}
		fmt.Fprintf(w, "%s %s: %s", ar.Kind, target, ar.Status)
		if ar.Detail != "" {
			fmt.Fprintf(w, " (%s)", ar.Detail)
		}
		fmt.Fprintln(w)
	}
	printCommentResults(w, out.Comments)
}
