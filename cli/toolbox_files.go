package cli

import (
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/version"
	"github.com/neokapi/neokapi/host/config"
	"github.com/spf13/cobra"
)

// FilesAliasName is the multi-call name of the project-free surface that the
// WP5 paired agent evaluation measures (docs/internals/evals.md): the same
// binary, exposing only inspect, apply, formats and the toolbox, with project
// discovery off. The name says what the surface works on and stays inside the
// kapi namespace, so it decides nothing about a product name (decision D14 in
// docs/internals/edit-model.md). `make build` links no such name, kapi's help
// lists none, and the command reference documents none: the evaluation
// installs the link in each of its cells.
const FilesAliasName = "kapi-files"

// filesProjectFlag is the flag host.AddProjectFlag registers on a
// project-aware command.
const filesProjectFlag = "project"

// newFilesRoot builds the root command BusyboxRoot returns for FilesAliasName.
// Every command resolves no project: a -p is refused, KAPI_PROJECT is
// dropped, and KAPI_NO_PROJECT turns the upward walk off, so a file is read
// with the format detection finds and named as the caller named it, wherever
// a kapi.yaml sits. Without a project nothing governs an edit: the commit
// check, the voice and the terms are what a project adds.
func newFilesRoot(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:     FilesAliasName,
		Short:   "Read and edit the content inside files of any format, with no project",
		Version: version.Version,
		Long: `Read any format into content blocks, edit them through a change set, and
write each file back in its own format. Every command works on the files it
is given and resolves no project: no recipe, no terms, no voice and no check
at commit.

  inspect   read files into blocks, with the reference and revision an edit names
  apply     apply a kapi.change/v1 change set
  formats   list the formats and what each can do
  kcat, kgrep, ksed, kdiff, kconv
            read, search, rewrite, compare and convert the content of files`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	AddPersistentFlags(app, root)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if f := cmd.Flags().Lookup(filesProjectFlag); f != nil && f.Changed {
			return WithExitCode(ExitUsage, fmt.Errorf("%s works without a project and takes no -p", FilesAliasName))
		}
		if err := os.Setenv(project.NoProjectEnvVar, "1"); err != nil {
			return err
		}
		if err := os.Unsetenv(project.ProjectEnvVar); err != nil {
			return err
		}
		app.Config = config.NewAppConfig()
		if err := app.Init(); err != nil {
			return err
		}
		ApplyAppInitializers(app)
		return nil
	}
	root.PersistentPostRun = func(*cobra.Command, []string) { app.Shutdown() }

	apply := NewApplyCmd(app)
	filesApplyHelp(apply)
	cmds := []*cobra.Command{NewInspectCmd(app), apply, NewFormatsCmd(app)}
	for _, proxy := range NewToolboxProxies(app) {
		proxy.Hidden = false
		cmds = append(cmds, proxy)
	}
	for _, c := range cmds {
		c.GroupID = ""
		if f := c.Flags().Lookup(filesProjectFlag); f != nil {
			f.Hidden = true
		}
		root.AddCommand(c)
	}
	// No prompt is sent to a model by anything the alias runs.
	if f := root.PersistentFlags().Lookup("explain-prompts"); f != nil {
		f.Hidden = true
	}
	return root
}

// filesApplyHelp gives apply the help it has under the alias: content
// operations on the files a change set names, with no project, so no recipe,
// review decision or terms, and no check at commit for --gate to waive.
func filesApplyHelp(apply *cobra.Command) {
	apply.Short = "Apply a change set to the files it names"
	apply.Long = `Apply a kapi.change/v1 change set to the files it names, the write sibling
of 'kapi-files inspect'. A change set is one JSON object with its operations
under "ops", JSONL with the envelope fields on the first line and one
operation per line, or a JSON array of operations. It is read from CHANGESET
or, with no argument or "-", from standard input. 'kapi-files apply --schema'
prints its JSON Schema, and 'kapi-files apply --schema OP' the schema of
operation OP alone.

Each content operation names what it changes in "at" ({"doc", "block",
"edition"}, the "ref" kapi-files inspect prints for a block) and the revision
it read in "if_match" (the block's "rev"). set_content gives an edition new
content, and with "if_match": "absent" creates one. replace_text changes text
inside an edition by find, by offsets or by run positions, and keeps the
inline codes and plurals around it. An operation whose edition moved since it
was read is refused as stale with the current content, and one that would
drop, invent or unbalance an inline code is refused as guard. When any
operation is refused, nothing is written.

A document is named by its path as given. It holds one edition, in the
language its file or directory names (locales/nb.json, de/guide.md), else in
the source language; --out FILE writes the one edition a change set adds to a
document, a translation of it, to FILE. With no project nothing checks an edit
as it lands: read the result with 'kapi-files inspect' or 'kapi-files kcat'.

--dry-run computes the change set, writes nothing, and prints a diff per
document. --json prints the result (kapi.change-result/v1), also for a change
set that does not decode.

Exit status: 0 when the change set applied or previewed; 2 when it does not
decode or contradicts itself; 3 when an operation was refused, and nothing was
written.`
	apply.Example = `  kapi-files inspect docs/guide.md --jsonl
  kapi-files apply change.json
  kapi-files apply change.json --dry-run --json
  kapi-files apply translation.json --out locales/nb.json
  kapi-files apply --schema replace_text`
	if f := apply.Flags().Lookup("gate"); f != nil {
		f.Hidden = true
	}
	for name, usage := range map[string]string{
		"format":  "format of every document the change set names (default: detected from the file)",
		"out":     "the file to write the one edition the change set adds to a document, a translation of it",
		"schema":  "print the JSON Schema of a change set, or of the one operation named (kapi-files apply --schema set_attribute), and exit",
		"dry-run": "compute the change set, print a diff per document, and write nothing",
	} {
		if f := apply.Flags().Lookup(name); f != nil {
			f.Usage = usage
		}
	}
}
