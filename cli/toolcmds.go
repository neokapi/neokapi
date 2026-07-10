package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	coretools "github.com/neokapi/neokapi/core/tools"

	"github.com/mattn/go-isatty"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// TopLevelTools is the curated tier of registry tools that keep a visible
// top-level verb (surface strategy A3, extending the #1078 C1 folds). Every
// other CLI-visible tool mounts under `kapi tool <name>`, keeping a hidden
// one-release alias at the top level so existing invocations don't break.
// Presentation only: the registry's Category/Tags stay canonical and
// `kapi tools list` remains the discovery surface for the full set.
var TopLevelTools = map[string]bool{
	"translate":        true,
	"pseudo-translate": true,
	"qa":               true,
	"recycle":          true,
	"word-count":       true,
}

// NewToolCommands creates the CLI surface for the registry's CLI-visible
// tools: the curated TopLevelTools tier as visible top-level verbs, every
// other tool as a hidden one-release top-level alias, and the `kapi tool`
// group command hosting the full set. The registry stays the single source
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
		Use:     "tool",
		Short:   "Run any registry tool on files (the full set behind the curated top-level verbs)",
		GroupID: "advanced",
		Long: `Run any CLI-visible registry tool on files. The everyday tools (translate,
pseudo-translate, qa, recycle, word-count) also live at the top level; the
rest of the registry — format converters, checks, extractors, redaction — is
mounted here to keep 'kapi --help' navigable.

Discover tools and their options with 'kapi tools list' and
'kapi tools schema <name>'.`,
	}

	var cmds []*cobra.Command
	for _, entry := range entries {
		toolName := string(entry.Info.Name)
		if BespokeToolCommands[toolName] {
			continue // a dedicated command owns this verb (see BespokeToolCommands)
		}
		sub := newToolCommand(a, entry)
		sub.GroupID = "" // the tool group renders one flat list, not the root's help groups
		toolGroup.AddCommand(sub)
		top := newToolCommand(a, entry)
		if !TopLevelTools[toolName] {
			top = deprecatedAlias(top, fmt.Sprintf("note: `kapi %s` moved to `kapi tool %s`; this top-level alias will be removed in a future release.", toolName, toolName))
		}
		cmds = append(cmds, top)
	}

	return append(cmds, toolGroup)
}

// newToolCommand builds the cobra command for one CLI-visible registry tool.
// Called twice per tool — once for the `kapi tool` group and once for the
// top-level verb (visible for the curated TopLevelTools tier, hidden
// one-release alias otherwise) — so it must stay free of registration side
// effects.
func newToolCommand(a *App, entry registry.CLIToolEntry) *cobra.Command {
	toolName := string(entry.Info.Name)
	info := entry.Info
	ToolSchema := entry.Schema
	var formatMaps []string

	short := info.Description
	if short == "" {
		short = info.DisplayName
	}

	// Localization tools (translate, recycle, the bilingual checks,
	// pseudo-translate, …) collapse under one "Localization:" help group,
	// regardless of their schema Category. Everything else keeps its
	// category-named group. The schema Category is left untouched so docs
	// and the flow editor still see the canonical category.
	groupID := info.Category
	// Localization tools collapse under the "Localization:" group: those
	// explicitly tagged l10n, plus every translation-category tool (the
	// translation category has no own group — see schema.CategoryTranslation —
	// so an untagged MT/translate tool must still land here, not in a
	// now-undefined "translation" group).
	if HasTag(info.Tags, schema.TagL10n) || info.Category == schema.CategoryTranslation {
		groupID = "localization"
	}

	cmd := &cobra.Command{
		Use:     toolName + " [files...]",
		Aliases: info.Aliases,
		Short:   short,
		GroupID: groupID,
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
				case AllKLF(args):
					// KLF writers are locale-additive: reading and writing
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

			// Tools that require a TM (e.g. recycle) get a real SQLite
			// TM provider resolved from --tm or the project's .kapi/tm.db,
			// opened once and shared across every input file. Without this
			// the tool's config factory falls back to NullTMProvider and
			// leverages nothing. Mirrors the termbase glossary injection
			// below and reuses the flow path's TM opening logic.
			var tmProvider coretools.TMProvider
			if ToolRequires(ToolSchema, "tm") {
				p, cleanup, terr := a.OpenToolTM(cmd)
				if terr != nil {
					return terr
				}
				defer cleanup()
				tmProvider = p
			}

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
				// Tools that require a termbase (e.g. term-check) get the
				// project's bound glossary injected when no glossary was
				// supplied programmatically. This makes `kapi term-check
				// fr.json` enforce the project termbase with no flag.
				if ToolRequires(ToolSchema, "termbase") {
					if _, ok := config["glossary"]; !ok {
						glossary, gerr := a.ResolveProjectGlossary(cmd, effectiveLang)
						if gerr != nil {
							return nil, gerr
						}
						if len(glossary) > 0 {
							config["glossary"] = glossary
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
				t, terr := a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), config, effectiveLang)
				if terr != nil {
					return nil, terr
				}
				// The recycle config factory cannot read a non-JSON
				// provider from the config map (Provider is json:"-"), so it
				// defaults to NullTMProvider. Swap in the resolved SQLite TM
				// on the created tool's config so it actually leverages.
				// SourceLocale is also schema-hidden and never populated from
				// --source-lang by the factory, so the SQLite lookup would run
				// with an empty source locale and match nothing — set it from
				// the resolved source language so exact/fuzzy lookups hit.
				if tmProvider != nil {
					if cfg, ok := t.Config().(*coretools.TMLeverageConfig); ok {
						cfg.Provider = tmProvider
						if cfg.SourceLocale.IsEmpty() && a.SourceLang != "" {
							cfg.SourceLocale = model.LocaleID(a.SourceLang)
						}
					}
				}
				return t, nil
			}

			var collector func() flow.Collector
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
	cmd.Flags().Bool("pack", false, "when transforming a .klz, also eject the result to the .klz (auto-pack)")
	if info.WritesOutput {
		cmd.Flags().StringP("output", "o", "", "output path template (variables: {dir}, {name}, {ext}, {lang})")
		cmd.Flags().String("output-dir", "", "write outputs under DIR/{lang}/ (default: beside the input, mirroring its locale layout)")
	}
	RegisterSchemaFlags(cmd, ToolSchema)
	if ToolSchema.ToolMeta != nil {
		for _, req := range ToolSchema.ToolMeta.Requires {
			switch req {
			case "credentials":
				cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
			case "termbase":
				cmd.Flags().String("termbase", "", "named termbase or path to a glossary (defaults to the project termbase)")
			case "tm":
				cmd.Flags().String("tm", "", "named TM or path to a .db (defaults to the project TM at .kapi/tm.db)")
			}
		}
	}
	cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
	cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
	return cmd
}
