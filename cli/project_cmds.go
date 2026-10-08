package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/skills"
)

// NewInitCmd returns `kapi init`: scaffold a new kapi project in the current
// directory (or `--dir <path>`), with collections proposed from the files in
// the tree and the wiring the coding agents working there read. Idempotent:
// re-running on an existing project adopts it and reports content no
// collection reads yet.
func NewInitCmd(a *App) *cobra.Command {
	var (
		dir          string
		name         string
		sourceLocale string
		targetLocale []string
		framework    string
		presetName   string
		listPresets  bool
		noRulesFiles bool
		mintID       bool
		agents       string
	)
	cmd := &cobra.Command{
		Use:     "init",
		Short:   "Scaffold a new kapi project in the current directory",
		GroupID: "work",
		Long: `Create a kapi project here: a kapi.yaml recipe with collections for the
content kapi found in the tree, and the wiring coding agents read.

kapi init reads the files under the directory, honouring .gitignore and
skipping dependency, vendored and build directories, and writes a collection
for each kind of content it recognises: documents grouped by directory
(docs/**/*.md), a file at the root on its own (README.md), and i18n catalogs
where a framework preset's layout or a file named for the source language
marks them. Each collection carries a comment saying what it matched. The
result is the same every time for the same tree, and nothing is asked.

Run it again on a project that already has a recipe and it leaves your
collections as they are. It prints the content no collection reads yet, as
recipe lines to paste and as the 'kapi add' command that writes each one.

The project name defaults to the directory's basename and the source language
to en. --target-locale (repeatable) declares target languages, and catalogs
then get a target beside their source. --preset <name> (alias --framework)
writes a known stack's catalog layout even before its files exist;
--list-presets lists the presets.

The recipe gets a stable id under 'id:', which survives a rename, a move and a
clone. --mint-id writes one into an existing recipe that has none.

The project's context (its terms, voice and recorded decisions) lives in your
workspace, not in the checkout. kapi keeps a cache for this checkout in .kapi/,
which is ignored by version control and safe to delete.

For coding agents, kapi init writes an MCP server entry that starts
'kapi mcp' for this project (with --tools writing,translation when the recipe
declares target languages) and one short skill, SKILL.md, naming the four
habits kapi supports; 'kapi help <topic>' serves the rest. Claude Code is
always wired; Cursor, VS Code, Codex and .agents/ are wired where the project
already keeps their directory. --agents takes a comma-separated list
(claude-code, cursor, vscode, codex, agents), 'all' or 'none'. An MCP entry
someone else wrote is left as it is. In a skill directory an earlier kapi
filled, the files it copied there are removed and any other file is kept.
Codex reads its entry once you trust this repository there.

kapi init also writes the project's rules into AGENTS.md and CLAUDE.md, where
agents load them without asking: the root's for the rules that hold
everywhere, and one in each folder whose rules differ, such as a rename that
holds in help/ and not in api/. kapi owns one marked section of each file and
keeps it current as the context changes; your own text around it is kept.
--no-rules-files skips them, and 'kapi context sync --files-only' writes them
later.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// --list-presets: print the preset catalog and exit (absorbs the
			// former `kapi presets list`, #1078 C1).
			if listPresets {
				return PrintPresetList(cmd)
			}
			// --preset is the porcelain spelling of --framework.
			if presetName != "" {
				framework = presetName
			}
			root, err := ResolveDir(dir)
			if err != nil {
				return err
			}
			// Read before anything is written, so a name kapi does not know
			// fails the command rather than half-initializing a project.
			hosts, chosen, err := ParseAgentHosts(agents)
			if err != nil {
				return err
			}

			// `kapi init` is idempotent: re-running it (or running it on a
			// project that already has a recipe) is not an error. This lets
			// plugin contributions (e.g. connecting an existing kapi project to
			// a server) run on top of `kapi init` without a separate command:
			// `kapi init --server …` on an existing project just connects it.
			res, err := InitProject(root, InitOptions{
				Name:          name,
				SourceLocale:  sourceLocale,
				TargetLocales: targetLocale,
				Framework:     framework,
				MintID:        mintID,
				Formats:       a.FormatReg,
			})
			if err != nil {
				return err
			}

			if res.AlreadyInitialized {
				fmt.Fprintf(cmd.OutOrStdout(), "kapi project already initialized: %s\n", res.RecipePath)
				// Printed last, after the wiring and pointer lines, because it
				// ends with lines to paste.
				defer printUncovered(cmd, res.Uncovered)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized kapi project %q\n", res.Name)
				fmt.Fprintf(cmd.OutOrStdout(), "  recipe: %s\n", res.RecipePath)
				printProposed(cmd, res.Collections)
			}
			// The id is reported only where the user asked about it. An
			// ordinary init writes it into the recipe, which is where it is
			// read from.
			if mintID {
				printMintedID(cmd, res)
			}

			// The agent wiring follows the scaffold for the same reason the
			// rules files do: it is about the tools around the project
			// rather than the project, and a file it cannot write is reported
			// without undoing an init that succeeded.
			if !chosen {
				hosts = DetectAgentHosts(root)
			}
			wiring, werr := WriteAgentWiring(AgentWiringOptions{
				Root:    root,
				Hosts:   hosts,
				Recipe:  filepath.Base(res.RecipePath),
				Skills:  skills.Wiring(),
				Retired: skills.Retired,
				Tools:   MCPToolSets(res.TargetLanguages),
			})
			if werr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: agent wiring: %v\n", werr)
			} else {
				printAgentWiring(cmd, wiring)
			}

			if noRulesFiles {
				return nil
			}

			// The rules files are what an agent standing in this tree loads
			// without asking. They follow the scaffold rather than being part
			// of it because they read the project's context, which InitProject
			// has no reason to open. A file that cannot be written is reported
			// and does not undo an init that succeeded.
			files, ferr := a.WriteRulesFiles(CmdContext(cmd), res.RecipePath)
			if ferr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: rules files: %v\n", ferr)
				return nil
			}
			printInitRulesFiles(cmd, files, res.AlreadyInitialized)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to scaffold in (default: current directory)")
	cmd.Flags().StringVar(&name, "name", "", "Project name (default: directory basename)")
	cmd.Flags().StringVar(&sourceLocale, "source-locale", "en", "Source locale (BCP-47)")
	cmd.Flags().StringSliceVar(&targetLocale, "target-locale", nil, "Target locale (repeatable)")
	cmd.Flags().StringVar(&framework, "framework", "", "Write a known stack's catalog layout as collections (see 'kapi init --list-presets')")
	cmd.Flags().StringVar(&presetName, "preset", "", "Write a named framework preset's catalog layout as collections; alias of --framework")
	cmd.Flags().BoolVar(&listPresets, "list-presets", false, "List available presets (framework scaffolds and per-format parsing presets) and exit")
	cmd.Flags().BoolVar(&noRulesFiles, "no-rules-files", false, "Do not write the project's rules into AGENTS.md and CLAUDE.md")
	cmd.Flags().BoolVar(&mintID, "mint-id", false, "Write a stable project id into a recipe that has none, and print the id")
	cmd.Flags().StringVar(&agents, "agents", "", "Coding agents to wire this project for: a comma-separated list of claude-code, cursor, vscode, codex, agents; 'all'; or 'none' (default: claude-code plus every host already used here)")
	cmd.MarkFlagsMutuallyExclusive("preset", "framework")
	return cmd
}

// printMintedID reports the project's stable id and whether this run wrote it,
// so a user who asked for one can tell an id that just landed in their recipe
// from one that was already there.
func printMintedID(cmd *cobra.Command, res *InitResult) {
	switch {
	case res.IDMinted:
		fmt.Fprintf(cmd.OutOrStdout(), "  id:     %s (written to the recipe)\n", res.ID)
	case res.ID != "":
		fmt.Fprintf(cmd.OutOrStdout(), "  id:     %s (already set)\n", res.ID)
	}
}

// MCPToolSets returns the `kapi mcp --tools` sets kapi init writes into a
// project's MCP entry: the default writing set alone, which needs no flag, or
// the writing and translation sets when the recipe declares target languages.
// Either way the agent reaches the structured edit path, which the writing set
// holds (host/mcp_sets.go).
func MCPToolSets(targetLanguages []string) []string {
	if len(targetLanguages) == 0 {
		return nil
	}
	return []string{"writing", "translation"}
}

// printProposed lists the collections a new recipe was written with, or says
// that none was found and how to add one.
func printProposed(cmd *cobra.Command, proposals []ProposedCollection) {
	w := cmd.OutOrStdout()
	if len(proposals) == 0 {
		fmt.Fprintln(w, "  collections: none found; add one with 'kapi add <pattern>'")
		return
	}
	fmt.Fprintln(w, "  collections (proposed from the files here; edit the recipe to change them):")
	printProposalTable(w, proposals)
}

// printUncovered reports, on a re-run, the content no collection reads yet:
// the recipe lines that would read it, and the `kapi add` command that writes
// each. Nothing is written for them.
func printUncovered(cmd *cobra.Command, proposals []ProposedCollection) {
	if len(proposals) == 0 {
		return
	}
	w := cmd.OutOrStdout()
	fmt.Fprintln(w, "\nContent kapi can read that no collection covers yet:")
	printProposalTable(w, proposals)
	fmt.Fprintln(w, "\nTo read it, add these lines under collections: in the recipe:")
	fmt.Fprint(w, RenderCollections(proposals))
	fmt.Fprintln(w, "\nor run:")
	for _, p := range proposals {
		line := "  kapi add " + shellQuote(p.Path) + " --format " + p.Format
		if p.Target != "" {
			line += " --target " + shellQuote(p.Target)
		}
		fmt.Fprintln(w, line)
	}
}

// printProposalTable prints one line per proposal: its glob, its format and
// how many files it matched.
func printProposalTable(w io.Writer, proposals []ProposedCollection) {
	width := 0
	for _, p := range proposals {
		width = max(width, len(p.Path))
	}
	for _, p := range proposals {
		n := fmt.Sprintf("%d files", len(p.Files))
		if len(p.Files) == 1 {
			n = "1 file"
		}
		fmt.Fprintf(w, "    %-*s  %s, %s\n", width, p.Path, p.Format, n)
	}
}

// shellQuote single-quotes a pattern so a shell passes it to kapi unexpanded.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// printAgentWiring lists every file the agent wiring touched and what became
// of it. These files are loaded as configuration by a program that runs what
// they say, so the command that wrote them names each one.
//
// The label says which of the two a line is about, because they answer
// different questions: `mcp` is a server an agent host starts, `skill` is
// guidance it loads. `rules` labels the rules files.
func printAgentWiring(cmd *cobra.Command, res *AgentWiringResult) {
	w := cmd.OutOrStdout()
	for _, file := range res.Files {
		line := fmt.Sprintf("  %-7s %s (%s", string(file.Kind)+":", file.Path, file.Action)
		if file.Detail != "" {
			line += ": " + file.Detail
		}
		fmt.Fprintln(w, line+")")
		if len(file.Removed) > 0 {
			fmt.Fprintf(w, "          removed %s an earlier kapi copied there\n", countFiles(len(file.Removed)))
		}
		if len(file.Kept) > 0 {
			fmt.Fprintf(w, "          kept %s kapi did not write: %s\n", countFiles(len(file.Kept)), strings.Join(file.Kept, ", "))
		}
	}
	// Codex reads a repository's own configuration layer for a repository the
	// person has trusted, so the file kapi wrote starts answering on the first
	// session they open here rather than on the next command.
	if slices.Contains(res.Hosts, AgentHostCodex) {
		fmt.Fprintln(w, "  note:   Codex reads .codex/config.toml once you trust this repository there")
	}
}

// countFiles renders a file count.
func countFiles(n int) string {
	if n == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", n)
}

// printInitRulesFiles reports what init did to the rules files. On a fresh
// project every file is listed beside the recipe; on a re-run only a change is
// worth a line.
func printInitRulesFiles(cmd *cobra.Command, res *RulesFilesResult, rerun bool) {
	files := res.Files
	if rerun {
		files = res.Changed()
	}
	w := cmd.OutOrStdout()
	for _, f := range files {
		fmt.Fprintf(w, "  rules:  %s (%s)\n", f.Path, f.Action)
	}
}
