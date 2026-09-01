package host

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ExplainStderr is the --explain value meaning "render to stderr".
const ExplainStderr = "-"

// ApplySourceLocale threads the run's source language into a tool config.
//
// The AI prompts name the source language ("translate from en to fr"), but
// ToolConfigFactory carries only the *target* language, so nothing set
// sourceLocale and every prompt read "translate from  to fr" — the model was
// never told the source language. This is the one point every tool config passes
// through. Tools without the field ignore it (unknown JSON keys are dropped on
// the config round-trip).
//
// It lives here, exported, because the browser build replaces App.Init's
// preprocessor wholesale to force demo providers; without a shared helper the
// two would silently disagree about what reaches a prompt.
func ApplySourceLocale(sourceLang string, config map[string]any) map[string]any {
	if sourceLang == "" {
		return config
	}
	if config == nil {
		config = map[string]any{}
	}
	if _, ok := config["sourceLocale"]; !ok {
		config["sourceLocale"] = sourceLang
	}
	return config
}

// explainCollector accumulates the LLM exchanges of a run so they can be
// rendered once it finishes, rather than interleaved with the run's own output.
type explainCollector struct {
	mu        sync.Mutex
	exchanges []aiprovider.Exchange
	remove    func()
}

func (c *explainCollector) add(_ context.Context, e aiprovider.Exchange) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exchanges = append(c.exchanges, e)
}

func (c *explainCollector) all() []aiprovider.Exchange {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]aiprovider.Exchange(nil), c.exchanges...)
}

// StartExplain installs the process-wide LLM recorder when --explain is set, so
// every model call this run makes is captured — including calls made by plugin
// providers. No-op otherwise, and an ordinary run pays nothing.
func (a *App) StartExplain() {
	if a.Explain == "" {
		return
	}
	a.explain = &explainCollector{}
	a.explain.remove = aiprovider.AddRecorder(a.explain.add)
}

// FlushExplain renders the captured exchanges. It is called at the end of a run.
// A path other than ExplainStderr writes the exchanges as JSON, for diffing a
// prompt across runs or attaching it to a bug report.
func (a *App) FlushExplain() error {
	if a.explain == nil {
		return nil
	}
	a.explain.remove()

	exchanges := a.explain.all()
	// Shutdown can run more than once; render exactly once.
	a.explain = nil

	if a.Explain != ExplainStderr {
		f, err := os.Create(a.Explain)
		if err != nil {
			return fmt.Errorf("explain: %w", err)
		}
		defer f.Close()

		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(exchanges); err != nil {
			return fmt.Errorf("explain: %w", err)
		}
		fmt.Fprintf(os.Stderr, "explain: wrote %d LLM exchange(s) to %s\n", len(exchanges), a.Explain)
		return nil
	}

	RenderExplain(os.Stderr, exchanges)
	return nil
}

// RenderExplain writes a human-readable transcript of what was sent to the model
// and what came back.
func RenderExplain(w io.Writer, exchanges []aiprovider.Exchange) {
	if len(exchanges) == 0 {
		fmt.Fprintln(w, "explain: no LLM calls were made.")
		return
	}

	for i, ex := range exchanges {
		fmt.Fprintf(w, "\n─── LLM call %d/%d ─────────────────────────────────\n", i+1, len(exchanges))

		label := ex.Prompt
		if label == "" {
			label = "(no prompt id)"
		}
		fmt.Fprintf(w, "prompt:   %s", label)
		if ex.Version != "" {
			fmt.Fprintf(w, " (%s)", ex.Version)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "provider: %s", ex.Provider)
		if ex.Model != "" {
			fmt.Fprintf(w, "  model: %s", ex.Model)
		}
		fmt.Fprintln(w)
		if ex.Schema != nil {
			fmt.Fprintf(w, "schema:   %s (structured output)\n", ex.Schema.Name)
		}

		for _, m := range ex.Messages {
			fmt.Fprintf(w, "\n  ── %s ──\n", m.Role)
			if len(m.Sections) == 0 {
				// A prompt still built inline, with no section attribution.
				writeIndented(w, m.Text())
				continue
			}
			// Attribute each block of the prompt to what produced it, so a user
			// can see that the term rules came from their terms and the tag rule
			// from the framework — not one opaque wall of English.
			for i, s := range m.Sections {
				if i > 0 {
					fmt.Fprintln(w)
				}
				fmt.Fprintf(w, "  [%s · %s]\n", s.Kind, s.Origin)
				writeIndented(w, s.Render())
			}
		}

		if ex.Err != "" {
			fmt.Fprintf(w, "\n  ── error ──\n")
			writeIndented(w, ex.Err)
		} else if ex.Response != "" {
			fmt.Fprintf(w, "\n  ── response ──\n")
			writeIndented(w, ex.Response)
		}

		if u := ex.Usage; u.TotalTokens() > 0 {
			fmt.Fprintf(w, "\n  tokens: %d in, %d out\n", u.InputTokens, u.OutputTokens)
		}
	}

	var in, out int
	for _, ex := range exchanges {
		in += ex.Usage.InputTokens
		out += ex.Usage.OutputTokens
	}
	fmt.Fprintf(w, "\n─── %d LLM call(s), %d tokens in, %d out ───\n", len(exchanges), in, out)
}

// writeIndented prints a block of prompt text indented, so it is visually
// distinct from the surrounding report.
func writeIndented(w io.Writer, s string) {
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		fmt.Fprintf(w, "  %s\n", line)
	}
}
