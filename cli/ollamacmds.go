package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	aiprovider "github.com/neokapi/neokapi/providers/ai"

	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewOllamaCmd builds `kapi models ollama` — manage the local Ollama runtime that kapi
// drives for on-device, GPU-accelerated translation. Ollama is a separate
// install (it runs models on Metal/CUDA), but kapi handles everything downstream
// of it: detecting the server, listing models, and pulling the model a
// translation needs — so a user never has to leave kapi for a separate shell.
func NewOllamaCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ollama",
		Short: "Manage the local Ollama runtime for on-device translation",
		Long: "Detect, inspect, and feed the local Ollama runtime kapi uses for on-device\n" +
			"(GPU-accelerated) translation. Ollama itself is a one-time install from\n" +
			"https://ollama.com; kapi drives the rest: `kapi models ollama pull <model>` installs\n" +
			"a model, and `kapi translate --provider ollama --model <model>` uses it.",
	}
	cmd.PersistentFlags().String("url", "", "Ollama server URL (default $OLLAMA_HOST or "+aiprovider.DefaultOllamaBaseURL+")")
	cmd.AddCommand(newOllamaStatusCmd(a))
	cmd.AddCommand(newOllamaListCmd(a))
	cmd.AddCommand(newOllamaPullCmd(a))
	cmd.AddCommand(newOllamaInstallCmd(a))
	return cmd
}

func newOllamaStatusCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the Ollama runtime is installed, running, and which models are present",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base := OllamaBaseURL(cmd)
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

			out.NextStep = OllamaNextStep(out)
			return output.Print(cmd, out)
		},
	}
}

func newOllamaListCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List models installed on the Ollama server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := aiprovider.NewOllamaManager(OllamaBaseURL(cmd))
			models, err := mgr.List(cmd.Context())
			if err != nil {
				return err
			}
			rows := make([]output.OllamaModelRow, 0, len(models))
			for _, m := range models {
				rows = append(rows, output.OllamaModelRow{
					Name:      m.Name,
					SizeBytes: m.Size,
					Size:      HumanBytes(m.Size),
					Modified:  m.ModifiedAt,
				})
			}
			return output.Print(cmd, output.OllamaModelsOutput{Models: rows, Total: len(rows)})
		},
	}
}

func newOllamaPullCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "pull <model>",
		Short: "Download a model onto the Ollama server",
		Long: "Install a model so kapi can translate with it locally. <model> is any Ollama model\n" +
			"reference (e.g. llama3.2:3b, qwen3:1.7b, aya-expanse:8b). Progress is streamed; a\n" +
			"model already present is a no-op.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			mgr := aiprovider.NewOllamaManager(OllamaBaseURL(cmd))

			has, err := mgr.Has(cmd.Context(), name)
			if err != nil {
				return err
			}
			if has {
				return output.Print(cmd, output.OllamaPullOutput{Model: name, Action: "present"})
			}

			stderr := cmd.ErrOrStderr()
			progress := OllamaPullPrinter(stderr)
			if err := mgr.Pull(cmd.Context(), name, progress); err != nil {
				return err
			}
			return output.Print(cmd, output.OllamaPullOutput{Model: name, Action: "pulled"})
		},
	}
}

func newOllamaInstallCmd(a *App) *cobra.Command {
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

			fmt.Fprintln(stderr, OllamaInstallHint())
			if runtime.GOOS == "darwin" && brewErr == nil {
				fmt.Fprintln(stderr, "Or run `kapi models ollama install --run` to install it now.")
			}
			return nil
		},
	}
	cmd.Flags().Bool("run", false, "On macOS with Homebrew, run `brew install ollama` now")
	return cmd
}
