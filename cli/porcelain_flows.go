package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// Flow-backed porcelain: `kapi translate` and `kapi pseudo-translate` are
// top-level verbs that run built-in flows, not raw tools. The pitch command
// therefore carries the guardrails — content memory reuse and deterministic checks wrap
// the AI step — while every raw tool keeps exactly one spelling under
// `kapi exec <name>`. The verb ladder: `kapi up` brings a project up to date,
// `kapi translate` produces for files, `kapi exec` runs one bare tool.

// NewTranslateCmd creates `kapi translate`: run the built-in translate flow
// (recycle → translate → checks) over the given files.
func NewTranslateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "translate [files...]",
		Short:   "Translate files with guardrails: content memory reuse, AI translate, then checks",
		GroupID: "translate",
		Long: `Translate files through the built-in translate flow: leverage a bound
content memory first (a no-op without one), translate the rest with the
configured AI provider, then run the deterministic checks (placeholders,
inline tags, untranslated text) over what was produced.

This is the guardrailed spelling: the same three-step flow 'kapi up' loops
over a project, applied to ad-hoc files. The raw translate tool (no Memory pass,
no checks) stays available as 'kapi exec translate'. To bring a whole project
up to date, use 'kapi up'.`,
		Example: `  kapi translate messages.json --target-lang fr
  kapi translate docs/*.md --target-lang de -o out/{name}_{lang}.{ext}
  kapi translate app.xliff --target-lang ja --provider ollama`,
		Args: cobra.ArbitraryArgs,
		RunE: newPorcelainFlowRunE(a, "translate",
			"kapi translate needs input files: kapi translate <files...> --target-lang <locale>. To translate a whole project, use 'kapi up'"),
	}
	AddProjectFlag(cmd)
	a.AddFlowRunFlags(cmd)
	AddProgressFlag(cmd)
	return cmd
}

// NewPseudoTranslateCmd creates `kapi pseudo-translate`: run the built-in
// pseudo-translate flow over the given files — the keyless pre-flight test
// for locale readiness (expansion, accents, placeholder survival).
func NewPseudoTranslateCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pseudo-translate [files...]",
		Short:   "Pseudo-translate files to test locale readiness, with no AI keys needed",
		GroupID: "translate",
		Long: `Run the built-in pseudo-translate flow: expand every string with
locale-shaped accents and length padding so truncation, concatenation, and
hardcoded-string bugs surface before any real translation is bought. Runs
entirely offline, with no AI provider or keys required.

Use it as the pre-flight check; when the UI holds up, 'kapi translate' (ad
hoc) or 'kapi up' (project) produce the real translations.`,
		Example: `  kapi pseudo-translate messages.json
  kapi pseudo-translate src/locales/en.json -o src/locales/qps.json`,
		Args: cobra.ArbitraryArgs,
		RunE: newPorcelainFlowRunE(a, "pseudo-translate",
			"kapi pseudo-translate needs input files: kapi pseudo-translate <files...>"),
	}
	AddProjectFlag(cmd)
	a.AddFlowRunFlags(cmd)
	AddProgressFlag(cmd)
	return cmd
}

// newPorcelainFlowRunE builds the shared RunE for a flow-backed porcelain
// verb: positional files merge into the flow path's --input flag, project
// defaults apply when a recipe is discovered (same precedence as kapi run),
// and the named built-in flow executes.
func newPorcelainFlowRunE(a *App, flowName, needInputMsg string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		for _, f := range args {
			if err := cmd.Flags().Set("input", f); err != nil {
				return err
			}
		}
		inputs, _ := cmd.Flags().GetStringSlice("input")
		if len(inputs) == 0 {
			return errors.New(needInputMsg)
		}

		projectPath, err := ResolveProjectPath(cmd)
		if err != nil {
			return err
		}
		fallbackRunE := a.ResolveFallbackRunE(RunCmdOptions{})
		if projectPath != "" {
			return a.RunFromProject(cmd, flowName, projectPath, RunCmdOptions{FallbackRunE: fallbackRunE})
		}
		return a.RunFlow(cmd.Context(), cmd, flowName, FlowCmdOptions{FallbackRunE: fallbackRunE})
	}
}
