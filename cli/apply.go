package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/spf13/cobra"
)

// changeKind discriminates a change-set entry. `apply` is the single write verb:
// every deliberate, reviewed change Claude proposes — a content edit or an asset
// edit (glossary term, TM pair, brand rule, recipe field) — is one typed entry,
// so "is this change reviewed?" has one answer for everything and the backing
// stores are written by exactly one code path.
type changeKind string

const (
	kindContent changeKind = "content"
	kindTerm    changeKind = "term"
	kindTM      changeKind = "tm"
	kindBrand   changeKind = "brand"
	kindRecipe  changeKind = "recipe"
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

// NewApplyCmd builds `kapi apply`: the one write verb, the write sibling of
// `kapi inspect`. It reads a typed change-set and lands every entry — content
// edits through the byte-faithful format round-trip (drift- and inline-code
// guarded), asset edits into their committed source artifact followed by the
// existing compile into the gitignored cache. No AI provider is involved: Claude
// authored the changes; apply enforces the guardrails and writes them.
func (a *App) NewApplyCmd() *cobra.Command {
	var (
		diff   bool
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:     "apply [flags] [CHANGESET]",
		Short:   "Apply a typed change-set (content + asset edits) — the one write verb",
		GroupID: "processing",
		Long: `Apply a typed change-set: the write sibling of 'kapi inspect'. Each entry is
one reviewed change — a content edit, or an asset edit (glossary term, TM pair,
brand rule, recipe field). Content edits land through the same byte-faithful
round-trip 'kapi rewrite' uses (structure and inline codes preserved), drift-
guarded by content_hash; asset edits are written into their committed source
artifact and the existing import compiles them into the cache.

The change-set is JSONL (one entry per line), read from CHANGESET or, with no
argument or "-", from standard input. Content entries name their own file, so
apply writes those files in place; --diff previews the content changes and
writes nothing. No AI provider is required.`,
		Example: `  kapi inspect report.docx --jsonl | edit-the-text | kapi apply
  kapi apply changeset.jsonl
  kapi apply changeset.jsonl --diff
  kapi apply changeset.jsonl --in-place=.bak`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inPlace := cmd.Flags().Changed("in-place")
			if inPlace && diff {
				return errors.New("--diff previews changes without writing; it cannot be combined with -i/--in-place")
			}
			backupSuffix := ""
			if v, _ := cmd.Flags().GetString("in-place"); inPlace && v != sentinelInPlace {
				backupSuffix = v
			}
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			return a.runApply(cmd, path, diff, backupSuffix, asJSON)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&diff, "diff", false, "preview content changes as a unified diff and write nothing")
	f.BoolVar(&asJSON, "json", false, "print the apply report as JSON")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input/output format for content files (default: auto-detect)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input/output encoding")
	f.StringP("in-place", "i", "", "keep a backup of edited content files with --in-place=.bak")
	f.Lookup("in-place").NoOptDefVal = sentinelInPlace
	return cmd
}

func (a *App) runApply(cmd *cobra.Command, path string, diff bool, backupSuffix string, asJSON bool) error {
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
		case kindTerm, kindTM, kindBrand, kindRecipe:
			res := a.applyAssetEntry(ctx, cmd, e)
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
				fmt.Fprintf(cmd.ErrOrStderr(), "apply: %s: %v\n", displayName(file), derr)
			}
		} else {
			if derr := a.editDocument(ctx, file, t, "", true, backupSuffix, cmd.OutOrStdout()); derr != nil {
				if errors.Is(derr, context.Canceled) {
					return derr
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "apply: %s: %v\n", displayName(file), derr)
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
	if path == "" || path == stdinName {
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

	var entries []changeEntry
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var e changeEntry
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, fmt.Errorf("apply: parse change-set line %d: %w", line, err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// printApplyReport writes a short human summary of the apply outcome.
func printApplyReport(w io.Writer, out *applyOutput) {
	c := out.Content
	if n := len(c.Applied) + len(c.Skipped) + len(c.Stale) + len(c.GuardFailed); n > 0 {
		fmt.Fprintf(w, "content: %d applied, %d unchanged", len(c.Applied), len(c.Skipped))
		if len(c.Stale) > 0 {
			fmt.Fprintf(w, ", %d stale (source drifted — re-inspect)", len(c.Stale))
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
