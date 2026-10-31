package host

import (
	"bytes"
	"context"
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
// policy leaves it: the file is written again with each withheld value
// replaced by its visible token ("[REDACTED:Person]"), and every member the
// package holds (the skeleton, the raw source with --with-source, the
// content-memory matches) is taken from that redacted file. The originals go
// to a vault that stays with whoever extracted the package: the project's
// batch vault under .kapi/vault/batches/ for `kapi extract --format kpz`, and
// <package>.vault beside the package for `kapi extract <files> -o work.kpz`.
// The manifest records the vault's ID (kpz.Redaction), the vault file
// records the same ID, and kapi merge restores from that vault and from no
// other.
//
// A token reaches the redacted file as text, so it survives any format, and
// kapi merge restores it by its visible text within the block, as it does
// for a PO file. Whoever receives the package works on the tokens.

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

// newPackageVault writes an empty vault with the given ID at path. A vault
// already at path is replaced: it belonged to the package the extract
// replaces.
func newPackageVault(path, id string) error {
	if err := ensureVaultDir(path); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace vault %s: %w", path, err)
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
		return nil, fmt.Errorf("%s: its blocks are identified differently once the redaction policy withholds its values, so kapi merge could not restore them, and kapi writes no package. Extract XLIFF 2 or PO from a project instead (--format xliff2 or --format po)", in.doc)
	}

	v, err := redaction.OpenFileVault(in.vaultPath)
	if err != nil {
		return nil, err
	}
	rs := &redactedSource{data: data, blocks: redacted}
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
	// A value that reads as part of a token ("Person" in "[REDACTED:Person]")
	// is not held by the token.
	bare := data
	for _, tok := range tokens {
		bare = bytes.ReplaceAll(bare, []byte(tok), nil)
	}
	if heldIn(bare, rs.withheld) {
		return nil, fmt.Errorf("%s holds a value the redaction policy withholds outside any block kapi can withhold it from (in the file's markup, in a field kapi does not read as a block, or across an inline code), and a .kpz package carries that part of the file whole, so kapi writes no package. Extract XLIFF 2 or PO from a project instead (--format xliff2 or --format po)", in.doc)
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

// heldInText is heldIn over a string.
func heldInText(s string, withheld []string) bool {
	for _, w := range withheld {
		if strings.Contains(s, w) {
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
			return nil, fmt.Errorf("merge: %s is a redacted package: it holds a placeholder wherever its redaction policy withheld a value, and the vault that restores them is not at %s, so kapi merge writes nothing. %s", pkgName, path, regenerate)
		}
		return nil, fmt.Errorf("merge: %s is a redacted package, and its vault %s cannot be read: %w", pkgName, path, err)
	}
	v, err := redaction.OpenFileVault(path)
	if err != nil {
		return nil, fmt.Errorf("merge: %s is a redacted package, and its vault cannot be read: %w", pkgName, err)
	}
	if v.ID() != wantID {
		got := v.ID()
		if got == "" {
			got = "none"
		}
		return nil, fmt.Errorf("merge: %s is a redacted package that names vault %s, and the vault at %s is vault %s, so kapi merge writes nothing. %s", pkgName, wantID, path, got, regenerate)
	}
	return v, nil
}

// packageVaultHere reports whether the vault that restores the redacted
// package at pkgPath is on this machine: beside the package, or among the
// batch vaults of the project at recipe.
func packageVaultHere(recipe, pkgPath string, r *kpz.Redaction) bool {
	candidates := []string{workspaceVaultPath(pkgPath)}
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
