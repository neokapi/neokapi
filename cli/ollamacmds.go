package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/schema"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// NewOllamaCmd builds `kapi models ollama` — manage the local Ollama runtime that kapi
// drives for on-device, GPU-accelerated translation. Ollama is a separate
// install (it runs models on Metal/CUDA), but kapi handles everything downstream
// of it: detecting the server, listing models, and pulling the model a
// translation needs — so a user never has to leave kapi for a separate shell.
func (a *App) NewOllamaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ollama",
		Short: "Manage the local Ollama runtime for on-device translation",
		Long: "Detect, inspect, and feed the local Ollama runtime kapi uses for on-device\n" +
			"(GPU-accelerated) translation. Ollama itself is a one-time install from\n" +
			"https://ollama.com; kapi drives the rest — `kapi models ollama pull <model>` installs\n" +
			"a model, and `kapi translate --provider ollama --model <model>` uses it.",
	}
	cmd.PersistentFlags().String("url", "", "Ollama server URL (default $OLLAMA_HOST or "+aiprovider.DefaultOllamaBaseURL+")")
	cmd.AddCommand(a.newOllamaStatusCmd())
	cmd.AddCommand(a.newOllamaListCmd())
	cmd.AddCommand(a.newOllamaPullCmd())
	cmd.AddCommand(a.newOllamaInstallCmd())
	return cmd
}

// ollamaBaseURL resolves the server address from the --url flag, then the
// OLLAMA_HOST environment variable (Ollama's own convention), then the default.
func ollamaBaseURL(cmd *cobra.Command) string {
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

// ollamaInstallHint returns the platform-appropriate one-liner for installing
// the Ollama runtime.
func ollamaInstallHint() string {
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

func (a *App) newOllamaStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the Ollama runtime is installed, running, and which models are present",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := ollamaBaseURL(cmd)
			out := output.OllamaStatusOutput{BaseURL: base}

			if path, err := exec.LookPath("ollama"); err == nil {
				out.Installed = true
				out.BinaryPath = path
			}

			mgr := aiprovider.NewOllamaManager(base)
			if v, err := mgr.Version(cmd.Context()); err == nil {
				out.Running = true
				out.Version = v
				if models, err := mgr.List(cmd.Context()); err == nil {
					out.ModelCount = len(models)
					for _, m := range models {
						out.Models = append(out.Models, m.Name)
					}
				}
			}

			out.NextStep = ollamaNextStep(out)
			return output.Print(cmd, out)
		},
	}
}

// ollamaNextStep produces the guidance line for `kapi models ollama status` based on
// what is missing.
func ollamaNextStep(s output.OllamaStatusOutput) string {
	if !s.Installed && !s.Running {
		return ollamaInstallHint() + " Then run `kapi models ollama status` again."
	}
	if !s.Running {
		return "Ollama is installed but not running. Start it with `ollama serve` (or launch the Ollama app)."
	}
	if s.ModelCount == 0 {
		return "Ready. Install a translation model with `kapi models ollama pull llama3.2:3b`."
	}
	return ""
}

func (a *App) newOllamaListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List models installed on the Ollama server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := aiprovider.NewOllamaManager(ollamaBaseURL(cmd))
			models, err := mgr.List(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]output.OllamaModelRow, 0, len(models))
			for _, m := range models {
				rows = append(rows, output.OllamaModelRow{
					Name:      m.Name,
					SizeBytes: m.Size,
					Size:      humanBytes(m.Size),
					Modified:  m.ModifiedAt,
				})
			}
			return output.Print(cmd, output.OllamaModelsOutput{Models: rows, Total: len(rows)})
		},
	}
}

func (a *App) newOllamaPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull <model>",
		Short: "Download a model onto the Ollama server",
		Long: "Install a model so kapi can translate with it locally. <model> is any Ollama model\n" +
			"reference (e.g. llama3.2:3b, qwen3:1.7b, aya-expanse:8b). Progress is streamed; a\n" +
			"model already present is a no-op.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			mgr := aiprovider.NewOllamaManager(ollamaBaseURL(cmd))

			has, err := mgr.Has(cmd.Context(), name)
			if err != nil {
				return err
			}
			if has {
				return output.Print(cmd, output.OllamaPullOutput{Model: name, Action: "present"})
			}

			stderr := cmd.ErrOrStderr()
			progress := ollamaPullPrinter(stderr)
			if err := mgr.Pull(cmd.Context(), name, progress); err != nil {
				return err
			}
			return output.Print(cmd, output.OllamaPullOutput{Model: name, Action: "pulled"})
		},
	}
}

// ollamaPullPrinter renders streaming pull progress. On a terminal it redraws a
// single percentage line per layer; otherwise it logs one line per phase (and
// per 10% step) so CI logs stay readable. mpb is intentionally avoided so this
// file still compiles for GOOS=js.
func ollamaPullPrinter(w io.Writer) func(aiprovider.PullProgress) {
	f, isFile := w.(*os.File)
	tty := isFile && isatty.IsTerminal(f.Fd())
	var lastStatus string
	lastStep := -1
	return func(p aiprovider.PullProgress) {
		if p.Total > 0 {
			pct := int(p.Completed * 100 / p.Total)
			if tty {
				fmt.Fprintf(w, "\r%-12s %3d%% (%s / %s)        ", p.Status, pct, humanBytes(p.Completed), humanBytes(p.Total))
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

// ensureOllamaForTool is the translate-time preflight: when an AI tool will run
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
func (a *App) ensureOllamaForTool(cmd *cobra.Command, toolSchema *schema.ComponentSchema) error {
	if !toolRequires(toolSchema, "credentials") {
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

	base := ollamaBaseURL(cmd)
	mgr := aiprovider.NewOllamaManager(base)
	if _, err := mgr.Version(cmd.Context()); err != nil {
		return err // actionable "is it running?/install" guidance
	}

	stderr := cmd.ErrOrStderr()
	printer := ollamaPullPrinter(stderr) // one stateful renderer across all frames
	pulled, err := mgr.EnsureModel(cmd.Context(), model, printer)
	if err != nil {
		return err
	}
	if pulled {
		fmt.Fprintf(stderr, "✓ pulled %s\n", model)
	}
	return nil
}

func (a *App) newOllamaInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Print how to install the Ollama runtime (or install it on macOS with --run)",
		Long: "Show the platform-appropriate command to install the Ollama runtime. With --run on\n" +
			"macOS (and Homebrew present), kapi runs `brew install ollama` for you.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stderr := cmd.ErrOrStderr()
			if path, err := exec.LookPath("ollama"); err == nil {
				fmt.Fprintf(stderr, "Ollama is already installed (%s).\n", path)
				return nil
			}

			run, _ := cmd.Flags().GetBool("run")
			brewPath, brewErr := exec.LookPath("brew")
			if run && runtime.GOOS == "darwin" && brewErr == nil {
				fmt.Fprintln(stderr, "Running: brew install ollama")
				c := exec.CommandContext(cmd.Context(), brewPath, "install", "ollama")
				c.Stdout = cmd.OutOrStdout()
				c.Stderr = stderr
				if err := c.Run(); err != nil {
					return fmt.Errorf("brew install ollama: %w", err)
				}
				fmt.Fprintln(stderr, "✓ Ollama installed. Start it with `ollama serve`, then `kapi models ollama pull llama3.2:3b`.")
				return nil
			}

			fmt.Fprintln(stderr, ollamaInstallHint())
			if runtime.GOOS == "darwin" && brewErr == nil {
				fmt.Fprintln(stderr, "Or run `kapi models ollama install --run` to install it now.")
			}
			return nil
		},
	}
	cmd.Flags().Bool("run", false, "On macOS with Homebrew, run `brew install ollama` now")
	return cmd
}
