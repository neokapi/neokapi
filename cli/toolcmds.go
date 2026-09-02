package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewToolCommands creates the CLI surface for the registry's CLI-visible
// tools: every tool has exactly one spelling, `kapi exec <name>`. There is
// no curated top-level tier — the top-level verbs are porcelain (up,
// translate, pseudo-translate, check, stats, …), each a guardrailed
// workflow rather than a raw tool. The verb pairing mirrors flows:
// `kapi flows` lists / `kapi run <flow>` runs; `kapi tools` lists /
// `kapi exec <tool>` runs one tool. The registry stays the single source
// of truth for tool metadata.
func NewToolCommands(a *App) []*cobra.Command {
	if a.ToolReg == nil {
		return nil
	}

	entries := a.ToolReg.CLITools()

	// Sort by category then name for stable command ordering.
	slices.SortFunc(entries, func(a, b registry.CLIToolEntry) int {
		if a.Info.Category != b.Info.Category {
			if a.Info.Category < b.Info.Category {
				return -1
			}
			return 1
		}
		if a.Info.Name < b.Info.Name {
			return -1
		}
		if a.Info.Name > b.Info.Name {
			return 1
		}
		return 0
	})

	toolGroup := &cobra.Command{
		Use:     "exec",
		Short:   "Execute one registry tool on files (the raw layer under the porcelain verbs)",
		GroupID: "advanced",
		Long: `Execute any CLI-visible registry tool on files: one tool, one pass, no
guardrails around it. The top-level verbs are porcelain workflows over this
layer: 'kapi translate' wraps recycle → translate → checks, 'kapi up'
brings a whole project up to date, 'kapi check' verifies, 'kapi stats' measures.
Reach for exec when you want exactly one tool's behavior: a bare
'exec translate' with no Memory pass, a single 'exec term-check', a format
converter. The verb pairs with 'kapi run <flow>' the way 'kapi tools' pairs
with 'kapi flows': run composes a pipeline, exec executes one tool.

Discover tools and their options with 'kapi tools list' and
'kapi tools schema <name>'.`,
		// Fail loudly on a tool name that doesn't exist (or no longer exists —
		// e.g. the retired count tools, whose job moved to `kapi stats`) instead
		// of silently printing help with exit 0.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}

	var cmds []*cobra.Command
	for _, entry := range entries {
		toolName := string(entry.Info.Name)
		if BespokeToolCommands[toolName] {
			continue // a dedicated command owns this verb (see BespokeToolCommands)
		}
		sub := newToolCommand(a, entry)
		sub.GroupID = "" // the exec group renders one flat list, not the root's help groups
		toolGroup.AddCommand(sub)
	}

	return append(cmds, toolGroup)
}

// newToolCommand builds the cobra command for one CLI-visible registry tool,
// mounted under the `kapi exec` group.
func newToolCommand(a *App, entry registry.CLIToolEntry) *cobra.Command {
	toolName := string(entry.Info.Name)
	info := entry.Info
	ToolSchema := entry.Schema
	var formatMaps []string

	short := info.Description
	if short == "" {
		short = info.DisplayName
	}

	cmd := &cobra.Command{
		Use:     toolName + " [files...]",
		Aliases: info.Aliases,
		Short:   short,
		Example: ToolExamples[toolName],
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool("json")
			jqFilter, _ := cmd.Flags().GetString("jq")
			jsonOut = jsonOut || jqFilter != "" // --jq implies JSON
			conc, _ := cmd.Flags().GetInt("concurrency")
			failUnknown, _ := cmd.Flags().GetBool("fail-on-unknown")
			strict, _ := cmd.Flags().GetBool("strict")
			failUnknown = failUnknown || strict
			noWarn, _ := cmd.Flags().GetBool("no-warn")
			progress, _ := cmd.Flags().GetBool("progress")

			mappings, err := ParseFormatMappings(formatMaps)
			if err != nil {
				return err
			}

			var outputTmpl string
			var inPlace bool
			var defaultLayout bool
			if info.WritesOutput {
				outputTmpl, _ = cmd.Flags().GetString("output")
				outputDir, _ := cmd.Flags().GetString("output-dir")
				switch {
				case outputTmpl != "":
					// Explicit -o template wins.
				case outputDir != "":
					// Root outputs under DIR using a locale-dir layout
					// (DIR/{lang}/<file>), mirroring tsc/babel --out-dir.
					outputTmpl = filepath.Join(outputDir, "{lang}") + string(filepath.Separator)
				case AllKBF(args):
					// KBF writers are locale-additive: reading and writing
					// back to the same file accumulates translations, so the
					// natural default is in-place.
					inPlace = true
				default:
					// Locale-aware default, resolved per file in the runner:
					// swap the source locale in the input path if present
					// (locales/en/app.json → locales/fr/app.json), else place
					// the file under a {lang}/ directory beside the input
					// (messages.json → fr/messages.json).
					defaultLayout = true
				}
			}

			effectiveLang := a.TargetLang
			if effectiveLang == "" && info.DefaultLocale != "" {
				effectiveLang = string(info.DefaultLocale)
			}

			tracePath, _ := cmd.Flags().GetString("trace")
			parallelBlocks, _ := cmd.Flags().GetInt("parallel-blocks")

			// A tool that requires or accepts a content memory gets one,
			// resolved from --memory or the project store, opened once and
			// shared across every input file. The grant is host.grantMemory, so
			// the CLI, a flow step and a direct build all hand a corpus over the
			// same way.
			grantCorpus, corpusCleanup := a.MemoryGrantFor(toolName, cmd)
			defer corpusCleanup()

			// First-run onboarding: when this tool needs provider credentials
			// and no AI provider is configured anywhere, walk through the
			// compact setup wizard inline on a TTY (kapi translate "just
			// works" on first use). Non-TTY runs keep the keys-only error.
			if ToolRequires(ToolSchema, "credentials") {
				if err := a.EnsureAIProviderInteractive(cmd); err != nil {
					return err
				}
			}

			// When this run targets the local Ollama provider, make sure the
			// runtime is up and the model is pulled before processing any
			// files — one clear up-front step instead of a per-block failure.
			if err := a.EnsureOllamaForTool(cmd, ToolSchema); err != nil {
				return err
			}

			newTool := func() (tool.Tool, error) {
				config := ReadAllSchemaFlags(cmd, ToolSchema)
				// Tools that require a terms store (e.g. term-check) get the
				// project's term rules injected when none were supplied
				// programmatically. This makes `kapi term-check fr.json`
				// enforce the project terms store with no flag.
				if ToolRequires(ToolSchema, schema.RequiresTerms) {
					if _, ok := config["term_rules"]; !ok {
						rules, gerr := a.ResolveTermRules(cmd, effectiveLang)
						if gerr != nil {
							return nil, gerr
						}
						if len(rules) > 0 {
							config["term_rules"] = rules
						}
					}
				}
				// Drop the flag/schema default provider+model when the user did
				// NOT explicitly set them, so a configured default can take
				// effect (project recipe defaults, then the app-config
				// ai.provider/ai.model applied by the registry preprocessor).
				// Without this, the flag's "anthropic" default would always sit
				// in the config and mask the configured default. When nothing is
				// configured, AI tools still fall back to their schema default
				// downstream, so behavior is unchanged for the no-config case.
				if f := cmd.Flags().Lookup("provider"); f != nil && !cmd.Flags().Changed("provider") {
					delete(config, "provider")
				}
				if f := cmd.Flags().Lookup("model"); f != nil && !cmd.Flags().Changed("model") {
					delete(config, "model")
				}
				credName, _ := cmd.Flags().GetString("credential")
				if credName != "" {
					config["credential"] = credName
				}
				if !jsonOut && isatty.IsTerminal(os.Stderr.Fd()) {
					config["onProgress"] = AiProgressWriter(os.Stderr)
				}
				config = grantCorpus(config)
				t, terr := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, effectiveLang)
				if terr != nil {
					return nil, terr
				}
				return t, nil
			}

			// A tool that produces check findings reports them at the end of the
			// run; without this a `kapi exec <check>` run annotated the blocks in
			// memory, exited 0, and printed nothing (#1476). A bespoke entry in
			// CollectorFactories still wins.
			collector := NewFindingsCollectorFor(ToolSchema)
			// voice-infer produces a profile rather than findings, and one for
			// the whole corpus rather than one per file, so it collects its own
			// way. See host/voiceinfer.go.
			if vc := NewVoiceInferCollectorFor(toolName, newTool); vc != nil {
				collector = vc
			}
			if cf, ok := CollectorFactories[toolName]; ok {
				collector = cf
			}

			rc := ToolRunConfig{
				ToolName:       toolName,
				Files:          args,
				FormatMappings: mappings,
				Concurrency:    conc,
				JSONOutput:     jsonOut,
				JQ:             jqFilter,
				Colorize:       output.Colorize(cmd, cmd.OutOrStdout()),
				FailOnUnknown:  failUnknown,
				NoWarn:         noWarn,
				Progress:       progress,
				OutputTemplate: outputTmpl,
				InPlace:        inPlace,
				DefaultLayout:  defaultLayout,
				TargetLang:     effectiveLang,
				TracePath:      tracePath,
				ParallelBlocks: parallelBlocks,
				NewTool:        newTool,
				NewCollector:   collector,
			}
			if p, _ := cmd.Flags().GetBool("pack"); p {
				rc.Pack = true
			}

			if !jsonOut && isatty.IsTerminal(os.Stderr.Fd()) {
				rc.AfterTool = func() {
					fmt.Fprint(os.Stderr, "\r\033[K")
				}
			}

			return a.RunToolOnFiles(cmd.Context(), rc)
		},
	}
	a.AddProcessingFlags(cmd)
	cmd.Flags().StringArrayVarP(&formatMaps, "map", "m", nil, "map glob pattern to format (e.g. '*.docx=okf_openxml:test')")
	cmd.Flags().Bool("json", false, "output results as JSON")
	cmd.Flags().IntP("concurrency", "j", 0, "max parallel files (0 = auto)")
	cmd.Flags().Bool("fail-on-unknown", false, "exit with error if any file cannot be processed (default: skip with warning)")
	cmd.Flags().Bool("strict", false, "alias for --fail-on-unknown")
	cmd.Flags().Bool("no-warn", false, "suppress warnings for skipped files")
	cmd.Flags().BoolP("progress", "p", false, "show progress bar")
	cmd.Flags().Bool("pack", false, "when transforming a .kpz, also eject the result to the .kpz (auto-pack)")
	if info.WritesOutput {
		cmd.Flags().StringP("output", "o", "", "output path template (variables: {dir}, {name}, {ext}, {lang})")
		cmd.Flags().String("output-dir", "", "write outputs under DIR/{lang}/ (default: beside the input, mirroring its locale layout)")
	}
	RegisterSchemaFlags(cmd, ToolSchema)
	if ToolSchema.ToolMeta != nil {
		for _, req := range ToolSchema.ToolMeta.Requires {
			switch req {
			case schema.RequiresCredentials:
				cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
			case schema.RequiresTerms:
				cmd.Flags().String("termstore", "", "named terms or path to a terms store (defaults to the project terms store)")
			case schema.RequiresMemory:
				cmd.Flags().String("memory", "", "named memory or path to a .db (defaults to the project content memory)")
			}
		}
	}
	cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
	cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
	return cmd
}
