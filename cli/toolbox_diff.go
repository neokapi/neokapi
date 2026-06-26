package cli

// kdiff — the format-aware reimagining of `diff`. Where the classic `diff`
// aligns two files line by line over raw bytes, kdiff aligns the translatable
// *blocks* of any format kapi understands, so a reflowed .docx, a re-zipped
// container, or a reordered JSON catalog never shows up as a diff — only the
// prose that actually changed does.
//
// It runs in two modes:
//
//   - Revision diff (two files): what translatable content changed between two
//     versions. `kdiff old.docx new.docx`, `kdiff --target fr a.xliff b.xliff`.
//   - Coverage diff (one file + --target): compare a target translation against
//     the source within a single file — which blocks are untranslated or are
//     still a verbatim copy of the source. `kdiff --target fr messages.xliff`.
//
// Alignment picks itself (--by auto): keyed formats (JSON/XLIFF/PO/… with stable
// block IDs) align by ID, so added / removed / changed / reordered keys fall out
// directly; prose formats (docx/md/html, whose IDs are positional) align by an
// LCS over the block text. `--by id|content` forces either.
//
// Like the rest of the toolbox it is exposed as a `kdiff` busybox symlink and as
// the hidden `kapi diff` subcommand, and follows the classic diff exit-code
// contract: 0 when the inputs are equivalent, 1 when they differ, 2 on trouble.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/spf13/cobra"
)

// lcsMaxCells caps the LCS dynamic-programming table for content alignment
// (≈4000×4000 → 64 MB at int32). Beyond it kdiff falls back to a positional
// alignment and prints a note, rather than allocating without bound — the cap is
// never silent (see runRevisionDiff).
const lcsMaxCells = 16_000_000

// diffKind classifies one aligned block pair.
type diffKind string

const (
	diffEqual   diffKind = "equal"
	diffAdd     diffKind = "added"
	diffRemove  diffKind = "removed"
	diffChange  diffKind = "changed"
	diffMove    diffKind = "moved"
	diffCopied  diffKind = "identical"    // coverage: target is a verbatim copy of source
	diffMissing diffKind = "untranslated" // coverage: no (or empty) target
)

// diffBlock is one projected block on one side of a diff. ID is the block's
// alignment key — its semantic name (a JSON key path, an XLIFF resname/unit id)
// when it has one, else its raw block ID (which may be positional).
type diffBlock struct {
	ID   string
	Text string
}

// The block's alignment key comes from blockKey (verify.go): its semantic Name
// (JSON key path, XLIFF resname/unit id) when set, else its raw ID. Catalog
// readers store the key in Name and a positional "tu1"/"d1" in ID, so Name is
// the stable identity to align on.

// diffOp is one entry in the computed diff.
type diffOp struct {
	Kind  diffKind
	ID    string
	ANum  int // 1-based block number on side A (0 when absent)
	BNum  int // 1-based block number on side B (0 when absent)
	AText string
	BText string
}

// diffOptions carries the resolved command flags.
type diffOptions struct {
	by        string // auto | id | content
	targetLoc model.LocaleID
	brief     bool
	stat      bool
	json      bool
	color     bool
}

// newDiffCmd builds the diff command. Used as the standalone `kdiff` root and,
// via NewToolboxProxies, behind the detached `kapi diff` subcommand.
func (a *App) newDiffCmd() *cobra.Command {
	var (
		by        string
		targetLoc string
		brief     bool
		stat      bool
	)

	cmd := &cobra.Command{
		Use:     "diff [flags] FILE_A [FILE_B]",
		Short:   "Compare the text/content of files block by block",
		GroupID: "content",
		Long: `Compare the human-readable text inside any supported format, block by block,
rather than byte by byte. A reflowed Word .docx, a re-zipped container or a
reordered JSON catalog do not register as a diff — only the prose that actually
changed does.

Two modes:

  Revision diff (two files)   — what translatable content changed between two
                                versions of a document.
  Coverage diff (one file +   — compare a target translation against the source
  --target LOCALE)              within one file: which blocks are untranslated or
                                are still a verbatim copy of the source.

Alignment is chosen automatically: keyed formats (JSON, XLIFF, PO, … with stable
block IDs) align by ID, so added / removed / changed / reordered keys are
reported directly; prose formats align by the block text. Force either with
--by id or --by content.

Exit status is 0 when the inputs are equivalent, 1 when they differ, 2 on error.`,
		Example: `  kdiff old.json new.json
  kdiff report.docx report-v2.docx
  kdiff --target fr messages.xliff            # coverage: source vs French
  kdiff --target fr old.xliff new.xliff       # what changed in the French
  kdiff --by content draft.md final.md
  kdiff --json a.json b.json`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch by {
			case "", "auto", "id", "content":
			default:
				return fmt.Errorf("invalid --by %q: use auto, id, or content", by)
			}
			if by == "" {
				by = "auto"
			}
			colorMode, _ := cmd.Flags().GetString("color")
			jsonOut, _ := cmd.Flags().GetBool("json")
			return a.runDiff(cmd.Context(), args, diffOptions{
				by:        by,
				targetLoc: model.LocaleID(targetLoc),
				brief:     brief,
				stat:      stat,
				json:      jsonOut,
				color:     useColor(colorMode),
			})
		},
	}

	f := cmd.Flags()
	f.StringVar(&by, "by", "auto", "alignment strategy: auto, id, or content")
	f.StringVar(&targetLoc, "target", "", "compare the target translation for LOCALE (one file: a coverage report)")
	f.BoolVarP(&brief, "brief", "q", false, "report only whether the inputs differ, not the changes")
	f.BoolVar(&stat, "stat", false, "print a one-line summary of the changes before the diff")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input format (default: auto-detect by extension/content)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input encoding")
	f.String("color", "auto", "colorize the diff: auto, always, never")
	f.Bool("json", false, "emit the diff as JSON")
	return cmd
}

// runDiff dispatches to the revision (two-file) or coverage (one-file) mode.
func (a *App) runDiff(ctx context.Context, args []string, opts diffOptions) error {
	switch len(args) {
	case 1:
		if opts.targetLoc == "" {
			return errors.New("comparing a single file needs --target LOCALE (a coverage report of source vs translation); pass two files to diff two documents")
		}
		return a.runCoverageDiff(ctx, args[0], opts)
	case 2:
		return a.runRevisionDiff(ctx, args[0], args[1], opts)
	case 0:
		return errors.New("no files to compare")
	default:
		return errors.New("kdiff compares one file (with --target, a coverage report) or two files")
	}
}

// --- revision diff (two files) ------------------------------------------------

func (a *App) runRevisionDiff(ctx context.Context, fileA, fileB string, opts diffOptions) error {
	ablocks, _, err := a.collectDiffBlocks(ctx, fileA, opts.targetLoc)
	if err != nil {
		return err
	}
	bblocks, _, err := a.collectDiffBlocks(ctx, fileB, opts.targetLoc)
	if err != nil {
		return err
	}

	useID := false
	switch opts.by {
	case "id":
		useID = true
	case "content":
		useID = false
	default: // auto
		useID = keyedSide(ablocks) && keyedSide(bblocks)
	}

	var ops []diffOp
	alignment := "content"
	if useID {
		ops = diffByID(ablocks, bblocks)
		alignment = "id"
	} else {
		var capped bool
		ops, capped = diffByContent(ablocks, bblocks)
		if capped {
			fmt.Fprintf(os.Stderr, "kdiff: inputs too large for content alignment; fell back to positional comparison (use --by id if the format has stable keys)\n")
		}
	}

	sum := summarize(ops)
	differ := sum.changed+sum.added+sum.removed+sum.moved > 0

	if opts.json {
		return emitRevisionJSON(fileA, fileB, alignment, opts.targetLoc, ops, sum, differ)
	}
	if opts.brief {
		if differ {
			fmt.Printf("Files %s and %s differ\n", displayName(fileA), displayName(fileB))
			return ErrSilentExit
		}
		return nil
	}
	if !differ {
		return nil // identical — print nothing, exit 0 (diff convention)
	}

	if opts.stat {
		fmt.Println(sum.line())
	}
	printDiffHeader(fileA, fileB, opts.color)
	for _, op := range ops {
		printDiffOp(op, opts.color)
	}
	return ErrSilentExit // differ → exit 1
}

// collectDiffBlocks projects a file to its blocks for the given scope ("" =
// source, otherwise a target locale). A block is included when it has an ID or
// non-empty text, so an ID-keyed alignment can still see a block whose target is
// missing on one side.
func (a *App) collectDiffBlocks(ctx context.Context, file string, scope model.LocaleID) ([]diffBlock, string, error) {
	var out []diffBlock
	fmtName, err := a.streamBlocks(ctx, file, func(_ int, b *model.Block) error {
		var text string
		if scope == "" {
			text = b.SourceText()
		} else {
			text = b.TargetText(scope)
		}
		key := blockKey(b)
		if text == "" && key == "" {
			return nil
		}
		out = append(out, diffBlock{ID: key, Text: text})
		return nil
	})
	return out, fmtName, err
}

// --- coverage diff (one file vs its own translation) --------------------------

// covBlock is one block's source/target pairing for a coverage report.
type covBlock struct {
	ID     string
	Source string
	Target string
	HasTgt bool
}

func (a *App) runCoverageDiff(ctx context.Context, file string, opts diffOptions) error {
	blocks, err := a.collectCoverage(ctx, file, opts.targetLoc)
	if err != nil {
		return err
	}

	var ops []diffOp
	var translated, untranslated, identical int
	for i, b := range blocks {
		switch {
		case !b.HasTgt || b.Target == "":
			untranslated++
			ops = append(ops, diffOp{Kind: diffMissing, ID: b.ID, ANum: i + 1, AText: b.Source})
		case b.Target == b.Source:
			identical++
			ops = append(ops, diffOp{Kind: diffCopied, ID: b.ID, ANum: i + 1, AText: b.Source})
		default:
			translated++
		}
	}
	drift := untranslated + identical

	if opts.json {
		return emitCoverageJSON(file, opts.targetLoc, ops, translated, untranslated, identical)
	}

	summary := fmt.Sprintf("%s [%s]: %d translated, %d untranslated, %d identical to source",
		displayName(file), opts.targetLoc, translated, untranslated, identical)
	if opts.brief {
		fmt.Println(summary)
		if drift > 0 {
			return ErrSilentExit
		}
		return nil
	}
	if opts.stat || drift == 0 {
		fmt.Println(summary)
	}
	for _, op := range ops {
		printCoverageOp(op, opts.color)
	}
	if drift > 0 {
		return ErrSilentExit // pending translation work → exit 1
	}
	return nil
}

func (a *App) collectCoverage(ctx context.Context, file string, locale model.LocaleID) ([]covBlock, error) {
	var out []covBlock
	_, err := a.streamBlocks(ctx, file, func(_ int, b *model.Block) error {
		s := b.SourceText()
		has := b.HasTarget(locale)
		if s == "" && !has {
			return nil
		}
		out = append(out, covBlock{ID: blockKey(b), Source: s, Target: b.TargetText(locale), HasTgt: has})
		return nil
	})
	return out, err
}

// --- alignment: by ID ---------------------------------------------------------

// diffByID aligns two block sequences by their stable IDs. Blocks present on
// both sides with differing text are "changed"; equal-text blocks whose relative
// order moved are "moved"; the rest are "added" or "removed". Output is in B
// order (the new file), with removals appended in A order.
func diffByID(a, b []diffBlock) []diffOp {
	type entry struct {
		text string
		num  int
	}
	aIdx := make(map[string]entry, len(a))
	for i, blk := range a {
		aIdx[blk.ID] = entry{text: blk.Text, num: i + 1}
	}
	bIdx := make(map[string]bool, len(b))
	for _, blk := range b {
		bIdx[blk.ID] = true
	}

	moved := movedIDs(a, b, bIdx)

	var ops []diffOp
	for j, blk := range b {
		ae, ok := aIdx[blk.ID]
		if !ok {
			ops = append(ops, diffOp{Kind: diffAdd, ID: blk.ID, BNum: j + 1, BText: blk.Text})
			continue
		}
		switch {
		case ae.text != blk.Text:
			ops = append(ops, diffOp{Kind: diffChange, ID: blk.ID, ANum: ae.num, BNum: j + 1, AText: ae.text, BText: blk.Text})
		case moved[blk.ID]:
			ops = append(ops, diffOp{Kind: diffMove, ID: blk.ID, ANum: ae.num, BNum: j + 1, AText: ae.text, BText: blk.Text})
		default:
			ops = append(ops, diffOp{Kind: diffEqual, ID: blk.ID, ANum: ae.num, BNum: j + 1, AText: ae.text, BText: blk.Text})
		}
	}
	for i, blk := range a {
		if !bIdx[blk.ID] {
			ops = append(ops, diffOp{Kind: diffRemove, ID: blk.ID, ANum: i + 1, AText: blk.Text})
		}
	}
	return ops
}

// movedIDs returns the set of common IDs whose relative order changed between
// the two sides — i.e. those not on a longest common subsequence of the shared
// IDs in document order.
func movedIDs(a, b []diffBlock, bIdx map[string]bool) map[string]bool {
	aInB := func(blk diffBlock) bool { return bIdx[blk.ID] }
	aCommon := make([]string, 0, len(a))
	inA := make(map[string]bool, len(a))
	for _, blk := range a {
		inA[blk.ID] = true
	}
	for _, blk := range a {
		if aInB(blk) {
			aCommon = append(aCommon, blk.ID)
		}
	}
	bCommon := make([]string, 0, len(b))
	for _, blk := range b {
		if inA[blk.ID] {
			bCommon = append(bCommon, blk.ID)
		}
	}
	pairs, ok := lcsPairs(aCommon, bCommon)
	if !ok {
		return nil // too large to compute; treat nothing as moved
	}
	inPlace := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		inPlace[aCommon[p[0]]] = true
	}
	moved := make(map[string]bool)
	for _, id := range aCommon {
		if !inPlace[id] {
			moved[id] = true
		}
	}
	return moved
}

// --- alignment: by content (LCS over block text) ------------------------------

// diffByContent aligns two block sequences by an LCS over their text, then
// coalesces adjacent removals + additions into "changed" pairs. Empty-text
// blocks are dropped first (they carry no comparable prose). The bool reports
// whether the LCS was capped and a positional fallback used instead.
func diffByContent(a, b []diffBlock) ([]diffOp, bool) {
	af := nonEmpty(a)
	bf := nonEmpty(b)
	aT := texts(af)
	bT := texts(bf)

	pairs, ok := lcsPairs(aT, bT)
	if !ok {
		return positionalOps(af, bf), true
	}

	var raw []diffOp
	ai, bi := 0, 0
	for _, p := range pairs {
		for ai < p[0] {
			raw = append(raw, diffOp{Kind: diffRemove, ANum: ai + 1, AText: af[ai].Text})
			ai++
		}
		for bi < p[1] {
			raw = append(raw, diffOp{Kind: diffAdd, BNum: bi + 1, BText: bf[bi].Text})
			bi++
		}
		raw = append(raw, diffOp{Kind: diffEqual, ANum: ai + 1, BNum: bi + 1, AText: af[ai].Text, BText: bf[bi].Text})
		ai++
		bi++
	}
	for ai < len(af) {
		raw = append(raw, diffOp{Kind: diffRemove, ANum: ai + 1, AText: af[ai].Text})
		ai++
	}
	for bi < len(bf) {
		raw = append(raw, diffOp{Kind: diffAdd, BNum: bi + 1, BText: bf[bi].Text})
		bi++
	}
	return coalesceChanges(raw), false
}

// positionalOps aligns block-by-block by index — the bounded fallback when the
// LCS table would be too large. Equal text is "equal", differing text is
// "changed", and the longer side's tail is "added"/"removed".
func positionalOps(a, b []diffBlock) []diffOp {
	var ops []diffOp
	n := min(len(a), len(b))
	for i := range n {
		if a[i].Text == b[i].Text {
			ops = append(ops, diffOp{Kind: diffEqual, ANum: i + 1, BNum: i + 1, AText: a[i].Text, BText: b[i].Text})
			continue
		}
		ops = append(ops, diffOp{Kind: diffChange, ANum: i + 1, BNum: i + 1, AText: a[i].Text, BText: b[i].Text})
	}
	for i := n; i < len(a); i++ {
		ops = append(ops, diffOp{Kind: diffRemove, ANum: i + 1, AText: a[i].Text})
	}
	for i := n; i < len(b); i++ {
		ops = append(ops, diffOp{Kind: diffAdd, BNum: i + 1, BText: b[i].Text})
	}
	return ops
}

// coalesceChanges turns a run of removals immediately followed by additions into
// index-paired "changed" ops, mirroring how `diff` presents an in-place edit.
// Leftover removals / additions (unequal counts) stay as-is.
func coalesceChanges(ops []diffOp) []diffOp {
	var out []diffOp
	i := 0
	for i < len(ops) {
		if ops[i].Kind != diffRemove && ops[i].Kind != diffAdd {
			out = append(out, ops[i])
			i++
			continue
		}
		// Gather the maximal run of removals/additions.
		var rems, adds []diffOp
		for i < len(ops) && (ops[i].Kind == diffRemove || ops[i].Kind == diffAdd) {
			if ops[i].Kind == diffRemove {
				rems = append(rems, ops[i])
			} else {
				adds = append(adds, ops[i])
			}
			i++
		}
		paired := min(len(adds), len(rems))
		for k := range paired {
			out = append(out, diffOp{
				Kind: diffChange, ID: rems[k].ID,
				ANum: rems[k].ANum, BNum: adds[k].BNum,
				AText: rems[k].AText, BText: adds[k].BText,
			})
		}
		out = append(out, rems[paired:]...)
		out = append(out, adds[paired:]...)
	}
	return out
}

// lcsPairs returns the matched index pairs of a longest common subsequence of a
// and b. The bool is false when the DP table would exceed lcsMaxCells, so the
// caller can fall back rather than allocate without bound.
func lcsPairs(a, b []string) ([][2]int, bool) {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return nil, true
	}
	if n*m > lcsMaxCells {
		return nil, false
	}
	w := m + 1
	dp := make([]int32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i*w+j] = dp[(i+1)*w+(j+1)] + 1
			} else if dp[(i+1)*w+j] >= dp[i*w+(j+1)] {
				dp[i*w+j] = dp[(i+1)*w+j]
			} else {
				dp[i*w+j] = dp[i*w+(j+1)]
			}
		}
	}
	var pairs [][2]int
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case dp[(i+1)*w+j] >= dp[i*w+(j+1)]:
			i++
		default:
			j++
		}
	}
	return pairs, true
}

// --- keyed/positional heuristics ----------------------------------------------

// keyedSide reports whether a side's blocks carry stable semantic keys —
// non-empty, unique IDs that are not the framework's positional auto-IDs
// (e.g. plaintext "tu1", "tu2"). Auto alignment uses ID alignment only when both
// sides are keyed.
func keyedSide(blocks []diffBlock) bool {
	if len(blocks) == 0 {
		return false
	}
	seen := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		if b.ID == "" || seen[b.ID] {
			return false
		}
		seen[b.ID] = true
	}
	return !positionalSide(blocks)
}

// positionalAutoID matches an alpha prefix followed by digits ("tu12", "d3"),
// the shape readers use for position-encoding auto-IDs.
var positionalAutoID = regexp.MustCompile(`^[A-Za-z]*[0-9]+$`)

// positionalSide reports whether every ID is a positional auto-ID whose trailing
// number strictly increases — i.e. the IDs encode document order rather than
// identity, so reordering them would be meaningless.
func positionalSide(blocks []diffBlock) bool {
	prev := -1
	for _, b := range blocks {
		if !positionalAutoID.MatchString(b.ID) {
			return false
		}
		n, ok := trailingInt(b.ID)
		if !ok || n <= prev {
			return false
		}
		prev = n
	}
	return true
}

// trailingInt extracts the trailing integer of s (the digits after any alpha
// prefix). ok is false when s has no trailing digits.
func trailingInt(s string) (int, bool) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return 0, false
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return 0, false
	}
	return n, true
}

// --- helpers ------------------------------------------------------------------

func nonEmpty(blocks []diffBlock) []diffBlock {
	out := make([]diffBlock, 0, len(blocks))
	for _, b := range blocks {
		if b.Text != "" {
			out = append(out, b)
		}
	}
	return out
}

func texts(blocks []diffBlock) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.Text
	}
	return out
}

// diffSummary counts the ops by kind.
type diffSummary struct {
	added, removed, changed, moved, unchanged int
}

func summarize(ops []diffOp) diffSummary {
	var s diffSummary
	for _, op := range ops {
		switch op.Kind {
		case diffAdd:
			s.added++
		case diffRemove:
			s.removed++
		case diffChange:
			s.changed++
		case diffMove:
			s.moved++
		case diffEqual:
			s.unchanged++
		}
	}
	return s
}

func (s diffSummary) line() string {
	parts := []string{
		fmt.Sprintf("%d changed", s.changed),
		fmt.Sprintf("%d added", s.added),
		fmt.Sprintf("%d removed", s.removed),
	}
	if s.moved > 0 {
		parts = append(parts, fmt.Sprintf("%d moved", s.moved))
	}
	return strings.Join(parts, ", ")
}

func printDiffHeader(fileA, fileB string, color bool) {
	a := "--- a/" + displayName(fileA)
	b := "+++ b/" + displayName(fileB)
	if color {
		fmt.Println(colorMatch + a + colorReset)
		fmt.Println(colorLineNum + b + colorReset)
		return
	}
	fmt.Println(a)
	fmt.Println(b)
}

func printDiffOp(op diffOp, color bool) {
	switch op.Kind {
	case diffEqual:
		return
	case diffChange:
		fmt.Println(diffHeaderLine(op, color))
		printDel(op.AText, color)
		printAdd(op.BText, color)
	case diffAdd:
		fmt.Println(diffHeaderLine(op, color))
		printAdd(op.BText, color)
	case diffRemove:
		fmt.Println(diffHeaderLine(op, color))
		printDel(op.AText, color)
	case diffMove:
		fmt.Println(diffHeaderLine(op, color))
	}
}

func diffHeaderLine(op diffOp, color bool) string {
	var label string
	if op.ID != "" {
		label = strconv.Quote(op.ID)
	} else {
		switch op.Kind {
		case diffAdd:
			label = fmt.Sprintf("block %d", op.BNum)
		case diffRemove:
			label = fmt.Sprintf("block %d", op.ANum)
		default:
			label = fmt.Sprintf("block %d", op.ANum)
		}
	}
	line := fmt.Sprintf("@@ %s (%s) @@", label, op.Kind)
	if color {
		return colorSep + line + colorReset
	}
	return line
}

func printDel(text string, color bool) {
	if color {
		fmt.Println(colorMatch + "- " + text + colorReset)
		return
	}
	fmt.Println("- " + text)
}

func printAdd(text string, color bool) {
	if color {
		fmt.Println(colorLineNum + "+ " + text + colorReset)
		return
	}
	fmt.Println("+ " + text)
}

func printCoverageOp(op diffOp, color bool) {
	label := "(" + string(op.Kind) + ")"
	if op.ID != "" {
		label = strconv.Quote(op.ID) + " " + label
	} else {
		label = fmt.Sprintf("block %d %s", op.ANum, label)
	}
	header := "@@ " + label + " @@"
	if color {
		header = colorSep + header + colorReset
	}
	fmt.Println(header)
	fmt.Println("  " + op.AText)
}

// --- JSON output --------------------------------------------------------------

type diffChangeJSON struct {
	Kind   string `json:"kind"`
	ID     string `json:"id,omitempty"`
	ANum   int    `json:"a_block,omitempty"`
	BNum   int    `json:"b_block,omitempty"`
	Source string `json:"source,omitempty"`
	Target string `json:"target,omitempty"`
}

type revisionJSON struct {
	Mode      string           `json:"mode"`
	FileA     string           `json:"file_a"`
	FileB     string           `json:"file_b"`
	Alignment string           `json:"alignment"`
	Locale    string           `json:"locale,omitempty"`
	Differ    bool             `json:"differ"`
	Summary   summaryJSON      `json:"summary"`
	Changes   []diffChangeJSON `json:"changes"`
}

type summaryJSON struct {
	Changed int `json:"changed"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Moved   int `json:"moved"`
}

func emitRevisionJSON(fileA, fileB, alignment string, loc model.LocaleID, ops []diffOp, sum diffSummary, differ bool) error {
	out := revisionJSON{
		Mode:      "revision",
		FileA:     displayName(fileA),
		FileB:     displayName(fileB),
		Alignment: alignment,
		Locale:    string(loc),
		Differ:    differ,
		Summary:   summaryJSON{Changed: sum.changed, Added: sum.added, Removed: sum.removed, Moved: sum.moved},
		Changes:   make([]diffChangeJSON, 0, len(ops)),
	}
	for _, op := range ops {
		if op.Kind == diffEqual {
			continue
		}
		out.Changes = append(out.Changes, diffChangeJSON{
			Kind: string(op.Kind), ID: op.ID, ANum: op.ANum, BNum: op.BNum,
			Source: op.AText, Target: op.BText,
		})
	}
	if err := writeJSON(out); err != nil {
		return err
	}
	if differ {
		return ErrSilentExit
	}
	return nil
}

type coverageJSON struct {
	Mode         string           `json:"mode"`
	File         string           `json:"file"`
	Locale       string           `json:"locale"`
	Translated   int              `json:"translated"`
	Untranslated int              `json:"untranslated"`
	Identical    int              `json:"identical"`
	Blocks       []diffChangeJSON `json:"blocks"`
}

func emitCoverageJSON(file string, loc model.LocaleID, ops []diffOp, translated, untranslated, identical int) error {
	out := coverageJSON{
		Mode: "coverage", File: displayName(file), Locale: string(loc),
		Translated: translated, Untranslated: untranslated, Identical: identical,
		Blocks: make([]diffChangeJSON, 0, len(ops)),
	}
	for _, op := range ops {
		out.Blocks = append(out.Blocks, diffChangeJSON{
			Kind: string(op.Kind), ID: op.ID, ANum: op.ANum, Source: op.AText,
		})
	}
	if err := writeJSON(out); err != nil {
		return err
	}
	if untranslated+identical > 0 {
		return ErrSilentExit
	}
	return nil
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
