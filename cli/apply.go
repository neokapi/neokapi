package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// NewApplyCmd builds `kapi apply`: the one write verb, the write sibling of
// `kapi inspect`. It reads a typed change-set and lands every entry — content
// edits through the byte-faithful format round-trip (drift- and inline-code
// guarded), asset edits into their committed source artifact followed by the
// existing compile into the gitignored cache. No AI provider is involved: Claude
// authored the changes; apply enforces the guardrails and writes them.
func NewApplyCmd(a *App) *cobra.Command {
	var (
		diff   bool
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:     "apply [flags] [CHANGESET]",
		Short:   "Apply a typed change-set (content + asset edits) — the one write verb",
		GroupID: "work",
		Long: `Apply a typed change-set: the write sibling of 'kapi inspect'. Each entry is
one reviewed change — a content edit, an asset edit (term, content-memory pair,
brand rule, recipe field), or a review decision (kind:"review"). Content edits
land through the same byte-faithful round-trip the engine's writers use (structure and
inline codes preserved), drift-guarded by content_hash; asset edits are written
into their committed source artifact and the existing import compiles them into
the cache; a review decision is recorded in the project state store.

A content memory pair (kind:"memory") is recycle leverage for future translation — it does not
promote a unit to reviewed. To approve a translated unit, use a kind:"review"
entry addressed by its file/id/locale (as 'kapi status --review' lists it), with
status "reviewed" (default) or "signed-off"; the decision is staged in the
project store and is bound to the translation's content hash, so a later edit
drops the unit back below reviewed. 'kapi commit' writes it into the committed
record under .kapi/units/.

The change-set is JSONL (one entry per line), read from CHANGESET or, with no
argument or "-", from standard input. Content entries name their own file, so
apply writes those files in place; --diff previews the content changes and
writes nothing. No AI provider is required.`,
		Example: `  kapi inspect report.docx --jsonl | edit-the-text | kapi apply
  kapi apply changeset.jsonl
  kapi apply changeset.jsonl --diff
  kapi status --review --json | approve-units | kapi apply
  kapi apply changeset.jsonl --in-place=.bak`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inPlace := cmd.Flags().Changed("in-place")
			if inPlace && diff {
				return errors.New("--diff previews changes without writing; it cannot be combined with -i/--in-place")
			}
			backupSuffix := ""
			if v, _ := cmd.Flags().GetString("in-place"); inPlace && v != SentinelInPlace {
				backupSuffix = v
			}
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			return a.RunApply(cmd, path, diff, backupSuffix, asJSON)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&diff, "diff", false, "preview content changes as a unified diff and write nothing")
	f.BoolVar(&asJSON, "json", false, "print the apply report as JSON")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "input/output format for content files (default: auto-detect)")
	f.StringVar(&a.SourceLang, "source-lang", "en", "source language (e.g. en, en-US)")
	f.StringVar(&a.Encoding, "encoding", "UTF-8", "input/output encoding")
	f.StringP("in-place", "i", "", "keep a backup of edited content files with --in-place=.bak")
	f.Lookup("in-place").NoOptDefVal = SentinelInPlace
	return cmd
}
