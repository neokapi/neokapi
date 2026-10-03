package cli

import (
	"fmt"

	"github.com/neokapi/neokapi/core/change"
	"github.com/spf13/cobra"
)

// NewApplyCmd builds `kapi apply`: the machine path for changing content. It
// reads a kapi.change/v1 change set and hands it to the change service, which
// applies every operation or none. No AI provider is involved: the sender
// wrote the change, and apply checks and lands it.
func NewApplyCmd(a *App) *cobra.Command {
	var (
		dryRun      bool
		asJSON      bool
		schema      bool
		printOps    bool
		gate        string
		out         string
		inPlaceFlag *InPlaceFlag
	)
	cmd := &cobra.Command{
		Use:     "apply [flags] [CHANGESET]",
		Short:   "Apply a change set: content edits, review decisions, terms and recipe fields",
		GroupID: "work",
		Long: `Apply a change set, the write sibling of 'kapi inspect'. A change set is
kapi.change/v1: one JSON object with its operations under "ops", JSONL with the
envelope fields on the first line and one operation per line, or a JSON array of
operations. It is read from CHANGESET or, with no argument or "-", from standard
input. 'kapi apply --schema' prints its JSON Schema, and 'kapi apply --schema
OP' the schema of operation OP alone (set_content, replace_text, set_attribute,
and so on).

Each content operation names what it changes in "at" ({"doc", "block",
"edition"}, the "ref" kapi inspect prints for a block) and the revision it read
in "if_match" (the block's "rev"). set_content gives an edition new content, in
the placeholder text a read shows or as runs, and with "if_match": "absent"
creates an edition. replace_text changes text inside an edition by find, by
code-point offsets or by run positions, and keeps the inline codes and plurals
around it. Offsets and run positions name the edition as it was read, even
after an earlier operation of the change set changed it; a find matches the
text as the earlier operations left it. A change set with no operation changes
nothing. An operation whose edition moved since it was read is refused as
stale with the current content. An edit that would drop, invent or unbalance an
inline code, or flatten a plural or select, is refused as guard. When any
operation is refused, nothing in the change set is written. An operation on the
translation a bilingual file (XLIFF, PO) holds names its language as the
edition, and a PO catalog is read in the one language the change set names.

decide records a review decision on the revision a person read. term, memory
and recipe write a term, a content-memory pair or a recipe field into the
project, after the content they refer to. kapi apply records every change as
the person or agent the environment names (see 'kapi help growing-context').
That holds for a change set a run printed with --print-ops too: the "origin"
its operations state (the tool that produced a translation) is not kept, and
kapi apply says so in one line.
A decision other than advise, a term, a content-memory pair and a recipe field
are a person's: run from an agent's shell they are refused.

A code comment is a block of its source file, keyed as 'kapi check' and 'kapi
inspect' report it (func/Parse), in Go and in the languages a comment plugin
reads. set_content rewrites it with its new prose in "text", without comment
markers, and "if_match" is the comment's "rev" from kapi inspect. Every byte
outside the comment stays as it is, the result must parse, and the language's
formatter must agree; what was written is checked again and its findings are
reported, and a failing one exits 3 unless --gate report is given. Running that
formatter runs code the project controls, so kapi asks once per project, in a
terminal, and records the answer. A change set edits code comments or
documents, never both.

Inside a project a document is named by its project-relative path. A file
outside the project is named by its absolute path, as kapi inspect names it, and
a change set that edits one edits nothing in the project. Outside a project a
document holds one edition, in the language --source-lang names or else the one
its file or directory names (locales/nb.json, or docs/de/guide.md beside
docs/en/guide.md), which a read prints in each ref; --out FILE writes the one
edition a change set adds to it, a translation, to FILE.

--dry-run computes and checks the change set, writes nothing, and prints a diff
per document. --gate report lands a change whose findings would otherwise
refuse it, for a person who has read them. --json prints the result
(kapi.change-result/v1), also for a change set that does not decode, whose
result carries the error. --print-ops prints the change set as decoded, with
its defaults filled in, and applies nothing.

Exit status: 0 when the change set applied or previewed; 2 when it does not
decode or contradicts itself; 3 when an operation was refused, and nothing was
written, when the change landed in part, or when the check of a written code
comment failed; 5 when a backend did not answer.`,
		Example: `  kapi inspect docs/guide.md --jsonl | write-the-edits | kapi apply
  kapi apply change.json
  kapi apply change.json --dry-run
  kapi apply change.json --json
  echo '{"ops":[{"op":"replace_text","at":{"doc":"docs/guide.md","block":"install/p"},"if_match":"<rev from kapi inspect>","edits":[{"find":"colour","text":"color"}]}]}' | kapi apply
  ksed 's/colour/color/g' docs/guide.md --print-ops | kapi apply
  kapi apply --schema
  kapi apply --schema set_attribute`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if schema {
				op := ""
				if len(args) == 1 {
					op = args[0]
				}
				return RunApplySchema(cmd.OutOrStdout(), op)
			}
			opts := ApplyOptions{DryRun: dryRun, JSON: asJSON, PrintOps: printOps, Out: out}
			switch gate {
			case "":
			case string(change.GateEnforce), string(change.GateReport):
				opts.Gate = change.Gate(gate)
			default:
				return WithExitCode(ExitUsage, fmt.Errorf("--gate: %q is not a gate; use enforce or report", gate))
			}
			if cmd.Flags().Changed("in-place") {
				opts.BackupSuffix = inPlaceFlag.Suffix
			}
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			return a.RunApply(cmd, path, opts)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "compute and check the change set, print a diff per document, and write nothing")
	f.StringVar(&gate, "gate", "", "what a failing finding the change introduces does: enforce (refuse it, the default) or report (land it with its findings; a person's choice)")
	f.BoolVar(&asJSON, "json", false, "print the result as JSON (kapi.change-result/v1)")
	f.BoolVar(&schema, "schema", false, "print the JSON Schema of a change set, or of the one operation named (kapi apply --schema set_attribute), and exit")
	f.BoolVar(&printOps, "print-ops", false, "print the change set as decoded, with its defaults filled in, and apply nothing")
	f.StringVar(&out, "out", "", "outside a project, the file to write the one edition the change set adds to a document (a translation of it); inside one, the recipe's target names it")
	f.StringVarP(&a.FormatFlag, "format", "f", "", "format of every document the change set names (default: what the recipe binds, else auto-detect)")
	a.AddEncodingFlag(f, "", "input/output encoding")
	inPlaceFlag = RegisterInPlace(f, "keep a copy of each file the change set replaces, with the SUFFIX given (--in-place=.bak)")
	// Asset operations are written into the project's committed sources, so
	// apply is a project verb and names its project the way every other one
	// does.
	AddProjectFlag(cmd)
	return cmd
}
