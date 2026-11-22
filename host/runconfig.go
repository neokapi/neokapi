package host

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
)

// runConfigFlag is one AI run flag and the step config key it sets.
type runConfigFlag struct {
	flag, key string
	// secret keeps the value out of every message.
	secret bool
}

// runConfigFlags are the flags a run carries into its AI steps
// (runFlagToolConfig). Every command that runs a flow registers them through
// AddRunConfigFlags, and every flow it runs takes them by the one rule
// seedRunConfig applies.
var runConfigFlags = []runConfigFlag{
	{flag: "credential", key: "credential"},
	{flag: "provider", key: "provider"},
	{flag: "api-key", key: "api_key", secret: true},
	{flag: "model", key: "model"},
	{flag: "instruction", key: "instruction"},
	{flag: "context", key: "context"},
	{flag: "batching", key: "batching"},
}

// AddRunConfigFlags registers the AI run flags on cmd.
func AddRunConfigFlags(cmd Command) {
	cmd.Flags().String("credential", "", "saved credential name to use (see 'kapi credentials list')")
	cmd.Flags().String("provider", "anthropic", providerFlagUsage())
	cmd.Flags().String("api-key", "", "API key for the AI provider")
	cmd.Flags().String("model", "", "AI model name")
	cmd.Flags().String("instruction", "", "extra guidance for the model while translating (e.g. \"informal register; keep product names in English\")")
	cmd.Flags().String("context", "", "what the model is told about a block besides the block: none, key (default), neighbours")
	cmd.Flags().String("batching", "", "how many blocks share one LLM call: auto (default), single")
}

func addTraceFlag(cmd Command) {
	cmd.Flags().String("trace", "", "write flow trace JSON to file (for flow visualization)")
}

func addStoreFlags(cmd Command) {
	cmd.Flags().String("memory", "", "named Memory for recycle flow (resolves from KAPI_HOME)")
	addTermstoreFlag(cmd)
}

func addTermstoreFlag(cmd Command) {
	cmd.Flags().String("termstore", "", "named terms store for term-lookup/enforce (resolves from KAPI_HOME)")
}

func addStrictFlags(cmd Command) {
	cmd.Flags().Bool("fail-on-unknown", false, "exit with error if any file cannot be processed (default: skip with a warning)")
	cmd.Flags().Bool("strict", false, "alias for --fail-on-unknown")
}

// refusePackWithoutKpz refuses --pack on a run whose input is not a .kpz
// package: the flag re-packs a transformed package and means nothing for any
// other input.
func refusePackWithoutKpz(cmd Command, inputPaths []string) error {
	if pack, _ := cmd.Flags().GetBool("pack"); pack && !kpzWorkspaceInput(inputPaths) {
		return errors.New("--pack applies to a .kpz input; this run reads no package")
	}
	return nil
}

func addConcurrencyFlag(cmd Command) {
	cmd.Flags().IntP("concurrency", "j", 0, "number of files to process at once (0 = auto)")
}

func addParallelBlocksFlag(cmd Command) {
	cmd.Flags().Int("parallel-blocks", 0, "fan out block processing across N goroutines (0 = off)")
}

// AddUpRunFlags registers the run flags `kapi up` honours: the AI run flags,
// the input encoding, --memory and --termstore, -j, --trace,
// --fail-on-unknown and --print-ops. The kapi-bowrain plugin's `up` registers
// the same set.
//
// The flags that pick files, formats, languages and outputs for one run (-i,
// -o, --target-lang, --source-lang, --format with its --engine, --explain,
// --pack) belong to `kapi run`; for `kapi up` the recipe and the config
// decide them. --parallel-blocks is left out because a convergence pass's
// wrapped tools cannot fan out.
func (a *App) AddUpRunFlags(cmd Command) {
	AddRunConfigFlags(cmd)
	a.AddEncodingFlag(cmd.Flags(), "e", "charset the input is read in; a byte-order mark in the file wins (default: the project's defaults.encoding, then UTF-8)")
	addStoreFlags(cmd)
	addConcurrencyFlag(cmd)
	addTraceFlag(cmd)
	addStrictFlags(cmd)
	cmd.Flags().Bool(printOpsFlag, false, printOpsUsage)
}

// upRecipeFlags are the flags `kapi up` does not take because the recipe
// decides what they would, each with the refusal naming the recipe key.
// --format read every file of a run as one format, re-reading the rest and
// writing their targets from what it misread; --engine came only with it.
var upRecipeFlags = map[string]string{
	"format": "--format does not apply to kapi up: set a collection's format in the recipe, under the collection's format: key (collections[].format.name)",
	"engine": "--engine does not apply to kapi up: the recipe selects the format engine, under defaults.engine (or defaults.formats.<name>.engine for one format)",
}

// UpFlagError is the error `kapi up` gives for a flag it does not parse: the
// refusal naming the recipe key for a flag the recipe decides, else err.
func UpFlagError(err error) error {
	for name, refusal := range upRecipeFlags {
		if strings.Contains(err.Error(), "unknown flag: --"+name) {
			return errors.New(refusal)
		}
	}
	return err
}

// ChangedRunConfigFlags names the AI run flags the person set on cmd, as
// they would type them (`--provider`).
func ChangedRunConfigFlags(cmd Command) []string {
	var set []string
	for _, f := range runConfigFlags {
		if cmd.Flags().Lookup(f.flag) != nil && cmd.Flags().Changed(f.flag) {
			set = append(set, "--"+f.flag)
		}
	}
	return set
}

// seedRunConfig applies the run flags to a flow's steps. It is the one
// precedence rule for every flow a run executes (a built-in flow, a recipe
// flow, a flow file, kapi up's default):
//
//  1. a key the step's own config sets,
//  2. the run flag,
//  3. the recipe's tool presets (defaults.locales, then defaults.tools),
//  4. KAPI_AI_PROVIDER / KAPI_AI_MODEL, then the stored ai.provider / ai.model,
//  5. the tool's own default.
//
// seedRunConfig places the flag under the step's own config; ApplyProjectBindings
// places the presets under both, and the tool resolves the rest. A step that
// names its own value keeps it, so a flow that compares two providers still
// compares them. Each value a step keeps over a flag is returned as a note,
// for the caller to print, so the flag is never silently dropped.
//
// The steps are copied: the recipe's own flow is never changed.
func seedRunConfig(steps []flow.FlowStep, runConfig map[string]any) ([]flow.FlowStep, []string) {
	if len(runConfig) == 0 {
		return steps, nil
	}
	var notes []string
	out := make([]flow.FlowStep, len(steps))
	for i, s := range steps {
		s.Config = mergeFlowNodeConfig(runConfig, s.Config)
		for _, f := range runConfigFlags {
			flagVal, given := runConfig[f.key]
			own, named := steps[i].Config[f.key]
			if !given || !named || fmt.Sprint(own) == fmt.Sprint(flagVal) {
				continue
			}
			name := s.Label
			if name == "" {
				name = s.Tool
			}
			if f.secret {
				notes = append(notes, fmt.Sprintf("step %q sets its own %s, so --%s does not apply to it", name, f.key, f.flag))
			} else {
				notes = append(notes, fmt.Sprintf("step %q sets %s %q, so --%s %q does not apply to it", name, f.key, fmt.Sprint(own), f.flag, fmt.Sprint(flagVal)))
			}
		}
		if len(s.Parallel) > 0 {
			var sub []string
			s.Parallel, sub = seedRunConfig(s.Parallel, runConfig)
			notes = append(notes, sub...)
		}
		out[i] = s
	}
	return out, notes
}

// SeedRunConfigSpec applies run configuration to a flow a surface resolved
// itself, by the rule seedRunConfig states, and returns the notes naming each
// step that kept its own value.
func SeedRunConfigSpec(spec *flow.StepsSpec, runConfig map[string]any) (*flow.StepsSpec, []string) {
	return seedRunConfigSpec(spec, runConfig)
}

// seedRunConfigSpec is seedRunConfig over a whole spec, copied.
func seedRunConfigSpec(spec *flow.StepsSpec, runConfig map[string]any) (*flow.StepsSpec, []string) {
	if spec == nil || len(runConfig) == 0 {
		return spec, nil
	}
	c := *spec
	var notes []string
	c.Steps, notes = seedRunConfig(spec.Steps, runConfig)
	return &c, slices.Compact(notes)
}

// RunFlagToolConfig is the run flags' tool config on cmd: the input every
// surface that runs or plans a flow hands to ResolveProjectFlow and
// ProjectFlowAISetup. A surface with no flags passes the settings its run
// was started with in the same shape, or nil.
func RunFlagToolConfig(cmd Command) map[string]any { return runFlagToolConfig(cmd) }
