package host

import (
	"fmt"
	"os"
	"slices"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// AllKBF returns true when every positional input path carries the
// `.kbf.json` extension. Used to decide whether a tool run defaults to
// in-place output (the KBF writer is locale-additive — accumulates
// target translations on each block) or the sibling `./out/...`
// template (every other format).
//
// The test must go through [format.HasExt]: `.kbf.json` is a compound suffix,
// and path/filepath.Ext reports only ".json" for one, so comparing its result
// to ".kbf.json" never matches and every KBF run silently loses its in-place
// default.
func AllKBF(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, p := range paths {
		if !format.HasExt(p, format.KBFExt) {
			return false
		}
	}
	return true
}

// HasTag reports whether a tool's freeform Tags include want (e.g.
// schema.TagL10n).
func HasTag(tags []string, want string) bool {
	return slices.Contains(tags, want)
}

// CollectorFactories maps tool names to streaming collector factories.
// Only tools that aggregate results across files need a collector.
var CollectorFactories = map[string]func() flow.Collector{}

// AiProgressWriter returns a ProgressEvent callback that writes a single
// rewriting status line to w. Thinking summaries and block counters are
// shown while running; the line is cleared when the final block completes.
func AiProgressWriter(w *os.File) func(aiprovider.ProgressEvent) {
	return func(e aiprovider.ProgressEvent) {
		if e.Thinking != "" {
			// Truncate long thinking summaries to fit a terminal line.
			think := e.Thinking
			if len(think) > 60 {
				think = think[:57] + "..."
			}
			if e.TotalBlocks > 0 {
				fmt.Fprintf(w, "\r\033[K  [%d/%d] thinking: %s", e.Block, e.TotalBlocks, think)
			} else {
				fmt.Fprintf(w, "\r\033[K  [%d] thinking: %s", e.Block, think)
			}
			return
		}
		// Block start or done — show/advance the counter. Rendering the start
		// event (not just done) means the line moves as soon as a slow on-device
		// generation begins, not only after it completes.
		if e.TotalBlocks > 0 {
			fmt.Fprintf(w, "\r\033[K  Translating [%d/%d]", e.Block, e.TotalBlocks)
		} else {
			fmt.Fprintf(w, "\r\033[K  Translating [%d]", e.Block)
		}
	}
}

// ToolExamples maps tool names to their cobra Example strings. Each entry is a
// newline-separated list of representative, runnable commands using the bundled
// playground fixtures (messages.json, app.xliff, page.html, etc.) so they work
// in the wasm CLI playground with no uploads.
//
// Registry tools mount under `kapi exec <tool>` (there is no `kapi qa`), so
// their examples carry the `exec` prefix. `translate` and `pseudo-translate`
// are the exception: they have dedicated top-level commands, so their examples
// keep the bare `kapi translate` / `kapi pseudo-translate` form.
//
// AI/MT commands use demo mode (no --provider flag needed in the playground).
var ToolExamples = map[string]string{
	// ── Quality ─────────────────────────────────────────────────────────
	"qa": `  kapi exec qa app.xliff --target-lang fr
  kapi exec qa app.xliff --target-lang fr --provider anthropic
  kapi exec qa app.xliff --target-lang de --json`,
	"term-check": `  kapi exec term-check app.xliff --source-lang en --target-lang fr
  kapi exec term-check messages.json --source-lang en --target-lang fr`,
	"voice-vocab-check": `  kapi exec voice-vocab-check docs/index.md
  kapi exec voice-vocab-check messages.json --source-lang en`,

	// ── Translation ─────────────────────────────────────────────────────
	"pseudo-translate": `  kapi pseudo-translate messages.json -o messages.pseudo.json
  kapi pseudo-translate app.xliff -o app.pseudo.xliff --target-lang qps`,
	"translate": `  kapi translate messages.json --target-lang fr
  kapi translate app.xliff --target-lang de --provider openai
  kapi translate app.xliff --target-lang de -o app.de.xliff`,
	"recycle": `  kapi exec recycle app.xliff --target-lang fr
  kapi exec recycle messages.json --target-lang de`,

	// ── Text Processing ─────────────────────────────────────────────────
	"search-replace": `  kapi exec search-replace messages.json --find "foo" --replace "bar"
  kapi exec search-replace page.html --find "colour" --replace "color"`,
	"case-transform": `  kapi exec case-transform messages.json --mode upper
  kapi exec case-transform messages.json --mode lower`,
	"segmentation": `  kapi exec segmentation messages.json
  kapi exec segmentation app.xliff`,

	// ── AI Quality ───────────────────────────────────────────────────────
	"review": `  kapi exec review app.xliff --target-lang fr
  kapi exec review messages.json --target-lang de`,
	"voice-check": `  kapi exec voice-check messages.json --target-lang fr
  kapi exec voice-check app.xliff --target-lang de`,

	// ── AI Analysis ───────────────────────────────────────────────────────
	"term-extract": `  kapi exec term-extract messages.json
  kapi exec term-extract app.xliff --provider anthropic`,
}

// BespokeToolCommands names tools that own a dedicated, hand-written top-level
// command (richer than the generic schema-driven one) and so must be skipped
// here to avoid a duplicate registration.
var BespokeToolCommands = map[string]bool{}
