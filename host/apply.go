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
)

// changeEntry is one line of a `kapi apply` change-set (JSONL; one entry per
// line). Only the fields relevant to its Kind are populated. Content edits carry
// the block address (file + id + content_hash) and the new placeholder-rendered
// text; asset edits carry an op and the per-asset fields.
type changeEntry struct {
	Kind changeKind `json:"kind"`

	// content
	File        string `json:"file,omitempty"`
	ID          string `json:"id,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Text        string `json:"text,omitempty"`

	// asset common
	Op string `json:"op,omitempty"`

	// term
	Term     string `json:"term,omitempty"`
	Locale   string `json:"locale,omitempty"`
	Status   string `json:"status,omitempty"`
	Replaces string `json:"replaces,omitempty"`

	// tm
	Source       string `json:"source,omitempty"`
	Target       string `json:"target,omitempty"`
	SourceLocale string `json:"source_locale,omitempty"`
	TargetLocale string `json:"target_locale,omitempty"`

	// brand
	List        string `json:"list,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	Severity    string `json:"severity,omitempty"`

	// recipe
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
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
}

func (o *applyOutput) ok() bool {
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

	var out applyOutput

	// Content entries grouped by file → one faithful round-trip per file.
	byFile := map[string][]changeEntry{}
	var fileOrder []string
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
		case kindTerm, kindMemory, kindVoice, kindRecipe:
			res := a.applyAssetEntry(ctx, cmd, e)
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

	for _, file := range fileOrder {
		report := &coretools.ApplyReport{}
		byID, byHash := buildEditMaps(byFile[file])
		t := coretools.NewApplyEditsTool(byID, byHash, report)
		if diff {
			if _, derr := a.rewriteDiffFile(ctx, file, t, cmd.OutOrStdout()); derr != nil {
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
}
