package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/kpz"
)

// A .kpz written under a redaction policy carries each source file as the
// policy leaves it. Extract writes the file again with each withheld value
// replaced by its visible token ("[REDACTED:Person]"), and every member of
// the package comes from that file. The originals go to a vault that stays
// with whoever extracted the package. The manifest and the vault file record
// the same ID, and kapi merge restores from that vault alone.
//
// The token is text in the redacted file, so it survives any format, and
// merge restores it by that text within the block, as it does for PO.

// workspaceVaultSuffix names the vault of a package an ad-hoc extract writes:
// the package's own path with this suffix.
const workspaceVaultSuffix = ".vault"

// workspaceVaultPath returns the vault beside the package at kpzPath.
func workspaceVaultPath(kpzPath string) string { return kpzPath + workspaceVaultSuffix }

// redactedSource is one source file as a redacted package carries it.
type redactedSource struct {
	// data is the file written with each withheld value replaced by its
	// visible token.
	data []byte
	// blocks are the file's blocks as read back from data.
	blocks []*model.Block
	// withheld are the originals the vault keeps for the file.
	withheld []string
	// detector finds what the policy's rules withhold, in any text; locale is
	// the language it reads the text in.
	detector redaction.Detector
	locale   model.LocaleID
}

// finds reports whether the policy's rules find a value to withhold in s,
// once each visible token in tokens is taken out of it.
func (rs *redactedSource) finds(ctx context.Context, s string, tokens []string) (bool, error) {
	for _, tok := range tokens {
		s = strings.ReplaceAll(s, tok, "")
	}
	matches, err := rs.detector.Detect(ctx, s, rs.locale)
	if err != nil {
		return false, fmt.Errorf("redaction: %w", err)
	}
	return len(matches) > 0, nil
}

// holds reports whether v, as JSON, holds a value the policy's rules
// withhold, or one the vault keeps for the file. A nil rs holds nothing.
func (rs *redactedSource) holds(ctx context.Context, v any) (bool, error) {
	if rs == nil {
		return false, nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return false, err
	}
	if heldIn(buf.Bytes(), rs.withheld) {
		return true, nil
	}
	return rs.finds(ctx, buf.String(), nil)
}

// policyDetector compiles the rules file a policy names into a detector.
// Relative paths are read against rootDir.
func policyDetector(spec *project.RedactionSpec, rootDir string) (redaction.Detector, error) {
	path := spec.Rules
	if path == "" {
		return nil, errors.New("redaction is enabled but no rules file is set")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootDir, path)
	}
	rf, err := redaction.LoadRulesFile(path)
	if err != nil {
		return nil, err
	}
	return redaction.NewRuleDetector(rf.Rules)
}

// joinedTexts returns the text of runs with inline codes left out
// (model.RunsText), and the same for every plural form and select case.
func joinedTexts(runs []model.Run) []string {
	out := []string{model.RunsText(runs)}
	for _, r := range runs {
		switch {
		case r.Plural != nil:
			for _, form := range r.Plural.Forms {
				out = append(out, joinedTexts(form)...)
			}
		case r.Select != nil:
			for _, c := range r.Select.Cases {
				out = append(out, joinedTexts(c)...)
			}
		}
	}
	return out
}

// runTexts returns the text of every text run in runs, in every plural form
// and select case too.
func runTexts(runs []model.Run) []string {
	var out []string
	for _, r := range runs {
		switch {
		case r.Text != nil:
			out = append(out, r.Text.Text)
		case r.Plural != nil:
			for _, form := range r.Plural.Forms {
				out = append(out, runTexts(form)...)
			}
		case r.Select != nil:
			for _, c := range r.Select.Cases {
				out = append(out, runTexts(c)...)
			}
		}
	}
	return out
}

// redactedSourceInput says which file redactSourceForKpz writes again and
// under which policy.
type redactedSourceInput struct {
	path      string
	formatID  registry.FormatID
	formatCfg map[string]any
	// doc names the file in the vault.
	doc       string
	spec      *project.RedactionSpec
	rootDir   string
	vaultPath string
	locale    model.LocaleID
}

// newPackageVault writes an empty vault with the given ID at path. It never
// replaces a vault already there, which is the only copy of the values an
// earlier extract withheld.
func newPackageVault(path, id string) error {
	if err := ensureVaultDir(path); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already holds the values an earlier extract withheld. Merge that package and remove the vault, or extract to another name", path)
	}
	v, err := redaction.OpenFileVault(path)
	if err != nil {
		return err
	}
	v.SetID(id)
	return v.Flush()
}

// redactSourceForKpz writes the file in again with the values its policy
// withholds replaced by their visible tokens, keeping the originals in the
// vault, and reads the result back. It refuses a file whose blocks are
// identified differently once the values are withheld, since merge restores
// by block, and a file that still holds a withheld value outside the blocks,
// since the package would carry it.
func (a *App) redactSourceForKpz(ctx context.Context, in redactedSourceInput) (*redactedSource, error) {
	if in.formatID == "" {
		return nil, fmt.Errorf("%s: kapi cannot tell the file's format, so it cannot withhold the values the redaction policy names", in.doc)
	}
	tmp, err := os.MkdirTemp("", "kapi-redact-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, filepath.Base(in.path))

	cfg := redactToolConfig(in.spec, in.rootDir, in.vaultPath, in.locale)
	cfg.Document = in.doc
	rt, err := tools.NewRedactTool(cfg)
	if err != nil {
		return nil, fmt.Errorf("redaction: %w", err)
	}
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: in.locale,
		Encoding:     a.InputEncoding(),
		DetectFormat: func(string) registry.FormatID { return in.formatID },
		ConfigureReader: func(r format.DataFormatReader, _ registry.FormatID) error {
			return project.ApplyReaderConfig(r, in.formatCfg)
		},
	})
	if err := runner.RunFile(ctx, "redact", []tool.Tool{rt, newShowTokensTool()}, in.path, out, ""); err != nil {
		return nil, fmt.Errorf("%s: withhold the values the redaction policy names: %w", in.doc, err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}

	original, _, err := project.ReadSourceBlocks(ctx, a.FormatReg, string(in.formatID), in.path, in.locale, "", in.formatCfg)
	if err != nil {
		return nil, fmt.Errorf("%s: read blocks: %w", in.doc, err)
	}
	redacted, _, err := project.ReadSourceBlocks(ctx, a.FormatReg, string(in.formatID), out, in.locale, "", in.formatCfg)
	if err != nil {
		return nil, fmt.Errorf("%s: read the redacted file back: %w", in.doc, err)
	}
	if !slices.Equal(readBlockIDs(original), readBlockIDs(redacted)) {
		return nil, fmt.Errorf("%s: withholding its values changes its block IDs, so kapi merge could not restore them. kapi wrote no package. Extract XLIFF 2 or PO instead (--format xliff2 or --format po)", in.doc)
	}

	det, err := policyDetector(in.spec, in.rootDir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", in.doc, err)
	}
	v, err := redaction.OpenFileVault(in.vaultPath)
	if err != nil {
		return nil, err
	}
	rs := &redactedSource{data: data, blocks: redacted, detector: det, locale: in.locale}
	var tokens []string
	for _, rv := range v.All() {
		if rv.Doc != in.doc {
			continue
		}
		if rv.Original != "" && !slices.Contains(rs.withheld, rv.Original) {
			rs.withheld = append(rs.withheld, rv.Original)
		}
		if rv.Disp != "" {
			tokens = append(tokens, rv.Disp)
		}
	}
	// The redacted file must hold nothing the rules withhold: not in its
	// markup, and not in a block, where a value split by an inline code
	// escapes the redact step. A value that reads as part of a token
	// ("Person" in "[REDACTED:Person]") is not held by the token.
	bare := data
	for _, tok := range tokens {
		bare = bytes.ReplaceAll(bare, []byte(tok), nil)
	}
	held := heldIn(bare, rs.withheld)
	if !held {
		if held, err = rs.finds(ctx, string(data), tokens); err != nil {
			return nil, err
		}
	}
	for _, b := range redacted {
		if held {
			break
		}
		// The text with its inline codes left out, so a value a code splits
		// ("Alice</b> Archer") reads whole, and each text run as it is.
		texts := append(joinedTexts(b.SourceRuns()), runTexts(b.SourceRuns())...)
		for _, text := range texts {
			if held, err = rs.finds(ctx, text, tokens); err != nil {
				return nil, err
			}
			if held {
				break
			}
		}
	}
	if held {
		return nil, fmt.Errorf("%s holds a withheld value outside any block (in markup, in a field kapi does not read, or across an inline code). kapi wrote no package. Extract XLIFF 2 or PO instead (--format xliff2 or --format po)", in.doc)
	}
	return rs, nil
}

func readBlockIDs(blocks []*model.Block) []string {
	ids := make([]string, 0, len(blocks))
	for _, b := range blocks {
		ids = append(ids, b.ID)
	}
	return ids
}

// heldIn reports whether data holds any of the withheld values.
func heldIn(data []byte, withheld []string) bool {
	for _, w := range withheld {
		if bytes.Contains(data, []byte(w)) {
			return true
		}
	}
	return false
}

// newShowTokensTool returns the step that draws each redaction placeholder
// in a block's source as its visible token, so the writer writes the token as
// text whatever the format.
func newShowTokensTool() tool.Tool {
	t := &tool.BaseTool{
		ToolName:        "show-redaction-tokens",
		ToolDescription: "Draws each redaction placeholder as its visible token",
	}
	t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		runs := v.SourceRuns()
		shown := redaction.ShowTokens(runs)
		if bytes.Equal(model.CanonicalRunsJSON(runs), model.CanonicalRunsJSON(shown)) {
			return tool.EditPlan{}, nil
		}
		return tool.EditPlan{NewRuns: shown, Edits: tokenEdits(runs, shown)}, nil
	}
	return t
}

// tokenEdits maps the source text of runs to that of shown, where shown
// draws each redaction placeholder of runs as its token. A placeholder adds
// nothing to the text, so each token is an insertion at the placeholder's
// place. A sequence holding a plural or a select reports one edit over its
// whole text, since the text of a form has no place in the sequence's own.
func tokenEdits(runs, shown []model.Run) []model.RunEdit {
	oldText, newText := model.RunsText(runs), model.RunsText(shown)
	if oldText == newText {
		return nil
	}
	if model.HasStructuredRuns(runs) || len(runs) != len(shown) {
		return []model.RunEdit{{Start: 0, End: utf8.RuneCountInString(oldText), NewLen: utf8.RuneCountInString(newText)}}
	}
	var edits []model.RunEdit
	at := 0
	for i, r := range runs {
		switch {
		case r.Text != nil:
			at += utf8.RuneCountInString(r.Text.Text)
		case r.Ph != nil && shown[i].Text != nil:
			edits = append(edits, model.RunEdit{Start: at, End: at, NewLen: utf8.RuneCountInString(shown[i].Text.Text)})
		}
	}
	return edits
}

// packageVault opens the vault that restores a redacted package and checks
// that it is the one the package names. pkgName is the package as the
// messages name it, path the vault file, and wantID the ID the manifest
// records. regenerate says how to get a vault that matches.
func packageVault(pkgName, path, wantID, regenerate string) (*redaction.FileVault, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("merge: %s is redacted, and its vault is not at %s. kapi merge wrote no file. %s", pkgName, path, regenerate)
		}
		return nil, fmt.Errorf("merge: %s is redacted, and its vault %s cannot be read: %w", pkgName, path, err)
	}
	v, err := redaction.OpenFileVault(path)
	if err != nil {
		return nil, packageVaultOpenError(pkgName, err)
	}
	if v.ID() != wantID {
		got := v.ID()
		if got == "" {
			got = "none"
		}
		return nil, fmt.Errorf("merge: %s is redacted and names vault %s. The vault at %s is vault %s. kapi merge wrote no file. %s", pkgName, wantID, path, got, regenerate)
	}
	return v, nil
}

// packageVaultOpenError is the refusal of a redacted package whose vault
// does not open. A vault a newer kapi wrote says to upgrade. One an earlier
// kapi wrote is refused with the vault named and the command to run
// (redaction.WithRun), and any other vault that does not open is replaced by
// extracting the package again.
func packageVaultOpenError(pkgName string, err error) error {
	if errors.Is(err, redaction.ErrOlderVault) {
		return redaction.WithRun(err, runForPackageVault)
	}
	if errors.Is(err, redaction.ErrNewerVault) {
		return fmt.Errorf("merge: %s is redacted, and its vault cannot be read: %w", pkgName, err)
	}
	return fmt.Errorf("merge: %s is redacted, and its vault cannot be read: %w. Run kapi extract again to write a new package and vault", pkgName, err)
}

// Advice the merge refusal of a redacted package ends with, per route.
const (
	workspaceVaultAdvice = "Copy the vault kapi extract wrote to that path, or extract the package again"
	projectVaultAdvice   = "Merge it in the project that extracted it, or extract it again"
)

// placeholderPrefixes returns the part of each visible token in entries that
// comes before its category ("[REDACTED:" of "[REDACTED:Person]"): text a
// translation holds only when a translator rewrote a placeholder.
func placeholderPrefixes(entries []redaction.RedactedValue) []string {
	var out []string
	for _, e := range entries {
		if e.Disp == "" {
			continue
		}
		prefix := e.Disp
		if cat := e.Category; cat != "" {
			title := strings.ToUpper(cat[:1]) + cat[1:]
			if i := strings.Index(e.Disp, title); i > 0 {
				prefix = e.Disp[:i]
			}
		}
		if !slices.Contains(out, prefix) {
			out = append(out, prefix)
		}
	}
	return out
}

// heldPlaceholder returns the first prefix that a text run of runs, in any
// plural form or select case, still holds.
func heldPlaceholder(runs []model.Run, prefixes []string) (string, bool) {
	for _, text := range runTexts(runs) {
		for _, p := range prefixes {
			if strings.Contains(text, p) {
				return p, true
			}
		}
	}
	return "", false
}

// errUnrestoredPlaceholder is the refusal of a translation that still holds
// a placeholder once the vault restored what it could.
func errUnrestoredPlaceholder(doc, blockID, prefix string) error {
	return &unrestoredPlaceholderError{doc: doc, block: blockID, prefix: prefix}
}

type unrestoredPlaceholderError struct{ doc, block, prefix string }

func (e *unrestoredPlaceholderError) Error() string {
	return fmt.Sprintf("merge: %s, block %s: the translation holds a placeholder no value in the vault matches (%q…), so a translator changed it. kapi merge wrote no file for it. Put the placeholder back as the package holds it and merge again", e.doc, e.block, e.prefix)
}

// packageVaultHere reports whether the vault that restores the redacted
// package r records is among the batch vaults of the project at recipe:
// only a package kapi extract --format kpz wrote in this project was
// redacted under the project's own policies.
func packageVaultHere(recipe string, r *kpz.Redaction) bool {
	var candidates []string
	if layout, err := project.LayoutFor(recipe); err == nil {
		candidates = append(candidates, layout.RedactionSidecarPath(r.VaultID))
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if v, err := redaction.OpenFileVault(path); err == nil && v.ID() == r.VaultID {
			return true
		}
	}
	return false
}

// packageRedaction returns the redaction record of the package at path, or
// nil when it records none or cannot be read.
func packageRedaction(path string) *kpz.Redaction {
	pkg, err := LoadWorkspace(path)
	if err != nil {
		return nil
	}
	return pkg.Redaction
}

// docPlaceholderPrefixes returns the placeholder prefixes of every value the
// vault keeps for doc.
func docPlaceholderPrefixes(vault redaction.Vault, doc string) []string {
	var entries []redaction.RedactedValue
	for _, rv := range vault.All() {
		if rv.Doc == doc {
			entries = append(entries, rv)
		}
	}
	return placeholderPrefixes(entries)
}

// newPlaceholderCheckTool returns the merge step that stops at a translation
// of doc into locale that still holds a placeholder once unredact restored
// what the vault keeps.
func newPlaceholderCheckTool(doc string, locale model.LocaleID, vault redaction.Vault) tool.Tool {
	prefixes := docPlaceholderPrefixes(vault, doc)
	t := &tool.BaseTool{
		ToolName:        "check-redaction-placeholders",
		ToolDescription: "Stops at a translation that holds a placeholder the vault cannot restore",
	}
	t.Annotate = func(v tool.BlockView) error {
		if p, ok := heldPlaceholder(v.TargetRuns(locale), prefixes); ok {
			return errUnrestoredPlaceholder(doc, v.ID(), p)
		}
		return nil
	}
	return t
}
