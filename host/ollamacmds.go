package host

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mattn/go-isatty"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/output"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// OllamaBaseURL resolves the server address from the --url flag, then the
// OLLAMA_HOST environment variable (Ollama's own convention), then the default.
func OllamaBaseURL(cmd Command) string {
	if v, _ := cmd.Flags().GetString("url"); v != "" {
		return normalizeOllamaHost(v)
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		return normalizeOllamaHost(v)
	}
	return aiprovider.DefaultOllamaBaseURL
}

// normalizeOllamaHost accepts the bare host[:port] form Ollama's OLLAMA_HOST
// often takes and upgrades it to a full URL.
func normalizeOllamaHost(v string) string {
	if !strings.Contains(v, "://") {
		return "http://" + v
	}
	return v
}

// OllamaInstallHint returns the platform-appropriate one-liner for installing
// the Ollama runtime.
func OllamaInstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("brew"); err == nil {
			return "Install it with `brew install ollama` (or download from https://ollama.com)."
		}
		return "Download and install Ollama from https://ollama.com/download."
	case "linux":
		return "Install it with `curl -fsSL https://ollama.com/install.sh | sh`."
	default:
		return "Download and install Ollama from https://ollama.com/download."
	}
}

// OllamaNextStep produces the guidance line for `kapi models ollama status` based on
// what is missing.
func OllamaNextStep(s output.OllamaStatusOutput) string {
	if !s.Installed && !s.Running {
		return OllamaInstallHint() + " Then run `kapi models ollama status` again."
	}
	if !s.Running {
		return "Ollama is installed but not running. Start it with `ollama serve` (or launch the Ollama app)."
	}
	if s.ModelCount == 0 {
		return "Ready. Install a translation model with `kapi models ollama pull llama3.2:3b`."
	}
	return ""
}

// OllamaPullPrinter renders streaming pull progress. On a terminal it redraws a
// single percentage line per layer; otherwise it logs one line per phase (and
// per 10% step) so CI logs stay readable. mpb is intentionally avoided so this
// file still compiles for GOOS=js.
func OllamaPullPrinter(w io.Writer) func(aiprovider.PullProgress) {
	f, isFile := w.(*os.File)
	tty := isFile && isatty.IsTerminal(f.Fd())
	var lastStatus string
	lastStep := -1
	return func(p aiprovider.PullProgress) {
		if p.Total > 0 {
			pct := int(p.Completed * 100 / p.Total)
			if tty {
				fmt.Fprintf(w, "\r%-12s %3d%% (%s / %s)        ", p.Status, pct, HumanBytes(p.Completed), HumanBytes(p.Total))
				if p.Completed >= p.Total {
					fmt.Fprintln(w)
				}
				return
			}
			if step := pct / 10; step != lastStep {
				lastStep = step
				fmt.Fprintf(w, "%s %d%%\n", p.Status, pct)
			}
			return
		}
		// Status-only frame (e.g. "pulling manifest", "verifying", "success").
		if p.Status != "" && p.Status != lastStatus {
			if tty && lastStatus != "" {
				fmt.Fprintln(w) // finish any in-progress \r line
			}
			lastStatus = p.Status
			lastStep = -1
			fmt.Fprintln(w, p.Status)
		}
	}
}

// EnsureOllamaForTool is the translate-time preflight: when an AI tool will run
// against the local Ollama provider, make sure the runtime is reachable and the
// requested model is installed — pulling it (with progress) the first time —
// before any blocks are processed. This turns the common failure modes (server
// down, model not pulled) into one clear up-front step instead of a per-block
// error, so `kapi translate --provider ollama --model llama3.2:3b file.json`
// just works on a fresh machine that has Ollama installed.
//
// It is a no-op unless the effective provider resolves to "ollama". Effective
// provider/model follow the same precedence as the run itself: an explicit
// --provider/--model flag, then the app-config ai.provider/ai.model default.
func (a *App) EnsureOllamaForTool(cmd Command, toolSchemaArg *schema.ComponentSchema) error {
	if !ToolRequires(toolSchemaArg, "credentials") {
		return nil // not a provider-backed AI tool
	}

	provider := ""
	if f := cmd.Flags().Lookup("provider"); f != nil && cmd.Flags().Changed("provider") {
		provider, _ = cmd.Flags().GetString("provider")
	} else if a.Config != nil {
		provider = a.Config.GetString(config.KeyAIProvider)
	}
	if provider != string(aiprovider.Ollama) {
		return nil
	}

	model := ""
	if f := cmd.Flags().Lookup("model"); f != nil && cmd.Flags().Changed("model") {
		model, _ = cmd.Flags().GetString("model")
	} else if a.Config != nil {
		model = a.Config.GetString(config.KeyAIModel)
	}
	if model == "" {
		model = aiprovider.DefaultOllamaModel
	}

	base := OllamaBaseURL(cmd)
	mgr := aiprovider.NewOllamaManager(base)
	if _, err := mgr.Version(cmd.Context()); err != nil {
		return err // actionable "is it running?/install" guidance
	}

	stderr := cmd.ErrOrStderr()
	printer := OllamaPullPrinter(stderr) // one stateful renderer across all frames
	pulled, err := mgr.EnsureModel(cmd.Context(), model, printer)
	if err != nil {
		return err
	}
	if pulled {
		fmt.Fprintf(stderr, "✓ pulled %s\n", model)
	}
	return nil
}
