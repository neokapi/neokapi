package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	aiprovider "github.com/neokapi/neokapi/providers/ai"

	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// NewModelsCmd builds `kapi models` — manage the large model assets that
// plugins declare in their manifest and that kapi (not the plugin) downloads,
// verifies, and caches in a shared location. See cli/modelassets.go.
func NewModelsCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "models",
		Short:   "Manage attached LLMs and ML models",
		GroupID: "assets",
		Long: "A single view of every model kapi can use, across four sources:\n\n" +
			"  • Detected:        keyless providers found on this machine, like the\n" +
			"                      Claude Code CLI (uses your Claude subscription)\n" +
			"  • Local · Ollama:  on-device models served by a local Ollama runtime\n" +
			"                      (`kapi models pull <model>` installs one)\n" +
			"  • Plugin models:   integrity-pinned assets a plugin downloads and caches\n" +
			"                      under $XDG_CACHE_HOME/kapi/models/<plugin>/<id>/<version>/\n" +
			"  • Cloud providers: remote models that require an API key\n\n" +
			"`kapi models setup` interactively picks and verifies the default provider.\n" +
			"`kapi models pull`/`prune` install and remove Ollama and plugin models; cloud\n" +
			"models are listed for reference. Filter with `--provider <ollama|plugin|cloud-id>`.\n" +
			"`kapi models ollama` manages the local Ollama runtime itself.",
	}
	cmd.AddCommand(newModelsListCmd(a))
	cmd.AddCommand(newModelsSetupCmd(a))
	cmd.AddCommand(newModelsDefaultCmd(a))
	cmd.AddCommand(newModelsPullCmd(a))
	cmd.AddCommand(newModelsPruneCmd(a))
	cmd.AddCommand(NewOllamaCmd(a))
	return cmd
}

// newModelsDefaultCmd builds `kapi models default` — show or set the default AI
// model (and the provider it implies). The default is the shared ai.provider /
// ai.model app config that both the CLI and Kapi Desktop honor when a run names
// no provider.
func newModelsDefaultCmd(a *App) *cobra.Command {
	var providerFlag string
	cmd := &cobra.Command{
		Use:   "default [model]",
		Short: "Show or set the default AI model (and its provider)",
		Long: "With no argument, prints the configured default AI provider and model.\n\n" +
			"With a model argument, sets it as the default used by AI tools and flows\n" +
			"when no --provider/--model flag or recipe default is given. The provider is\n" +
			"inferred from the model name by convention (claude-* → anthropic, gpt-* →\n" +
			"openai, gemini-* → gemini, an Ollama tag like gemma3:4b → ollama); pass\n" +
			"--provider to choose it explicitly (e.g. Azure OpenAI). Writes ai.provider\n" +
			"and ai.model to the global config (see `kapi config path`), so the setting\n" +
			"is shared with Kapi Desktop.",
		Example: "  kapi models default                  # show the current default\n" +
			"  kapi models default gemma3:4b        # local Ollama, provider inferred\n" +
			"  kapi models default claude-sonnet-4  # anthropic, inferred\n" +
			"  kapi models default gpt-4o --provider azureopenai",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read and write both go through config.ResolveAIDefault /
			// config.SetAIDefault — the one resolver and the one writer every
			// surface shares. Reading the config directly here is what let the
			// read-back disagree with what every consumer actually resolved.
			if len(args) == 0 {
				return output.Print(cmd, ModelDefaultOutput{Default: config.ResolveAIDefault(a.Config)})
			}

			model := args[0]
			provider := providerFlag
			if provider == "" {
				inferred, ok := aiprovider.ProviderForModel(model)
				if !ok {
					return fmt.Errorf("could not infer a provider for model %q; pass --provider <%s>",
						model, strings.Join(aiprovider.ProviderNames(), "|"))
				}
				provider = string(inferred)
			}
			if err := config.SetAIDefault(a.Config, provider, model); err != nil {
				return err
			}
			return output.Print(cmd, ModelDefaultOutput{
				Default: config.ResolveAIDefault(a.Config),
				Set:     true,
			})
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "",
		"set the provider explicitly instead of inferring it from the model name")
	output.AddFlags(cmd.Flags())
	return cmd
}

func newModelsListCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every model kapi can use (Ollama, plugin assets, cloud providers)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, _ := cmd.Flags().GetString("provider")

			// Ollama models (best-effort: a short timeout so a missing runtime
			// degrades gracefully rather than hanging the listing). Skipped when a
			// filter excludes the Ollama source, so no network call is made.
			var installed []aiprovider.OllamaModelInfo
			ollamaReachable := true
			wantOllama := filter == "" || filter == output.ModelSourceOllama
			if wantOllama {
				ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
				mgr := aiprovider.NewOllamaManager(OllamaBaseURL(cmd))
				models, oerr := mgr.List(ctx)
				cancel()
				ollamaReachable = oerr == nil
				if ollamaReachable {
					installed = models
				}
			}

			rows := BuildModelRows(a.AllPluginModels(), installed, aiprovider.Providers(),
				aiprovider.ClaudeCodeDetected(), filter)

			if wantOllama && !ollamaReachable {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"note: Ollama not detected, so local models show as available to pull; run `kapi models ollama status`.")
			}
			return output.Print(cmd, output.ModelsListOutput{Models: rows, Total: len(rows)})
		},
	}
	cmd.Flags().String("provider", "", "Filter to one source/provider (e.g. ollama, anthropic, or a plugin name)")
	return cmd
}

func newModelsPullCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "pull <model>",
		Short: "Download a model (Ollama model, or a plugin asset)",
		Long: "Install a model so kapi can translate with it. A bare reference defaults to an\n" +
			"Ollama model (e.g. llama3.2:3b, qwen3:1.7b) and is pulled into the local Ollama\n" +
			"runtime. A plugin model id (e.g. sat-3l-sm), a plugin name, or plugin/model fetches\n" +
			"that plugin's host-owned asset instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			plugin, asset, err := a.ResolveModelRef(ref)
			if err != nil {
				// An explicit plugin/model that didn't resolve is an error; a bare
				// unknown reference defaults to an Ollama model.
				if strings.Contains(ref, "/") {
					return err
				}
				return a.PullOllamaModel(cmd, ref)
			}
			if asset.Bundled {
				return fmt.Errorf("%s/%s is bundled with the plugin, so there is nothing to fetch", plugin, asset.ID)
			}
			dir, err := EnsureModel(cmd.Context(), asset, ModelEnsureOptions{
				Plugin: plugin,
				Logf:   func(f string, a ...any) { fmt.Fprintf(cmd.ErrOrStderr(), f+"\n", a...) },
			})
			if err != nil {
				return err
			}
			return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourcePlugin, Plugin: plugin, Model: asset.ID, Dir: dir, Action: "ready"})
		},
	}
}

func newModelsPruneCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "prune <model>",
		Short: "Remove a model (Ollama model, or a cached plugin asset)",
		Long: "Delete a model from disk. A bare reference defaults to an Ollama model; a plugin\n" +
			"model id (e.g. sat-3l-sm), plugin name, or plugin/model removes that cached asset.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			plugin, asset, err := a.ResolveModelRef(ref)
			if err != nil {
				if strings.Contains(ref, "/") {
					return err
				}
				return a.PruneOllamaModel(cmd, ref)
			}
			if asset.Bundled {
				return fmt.Errorf("%s/%s is bundled with the plugin and cannot be removed", plugin, asset.ID)
			}
			dir, err := ModelDir(plugin, asset.ID, asset.Version)
			if err != nil {
				return err
			}
			if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
				return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourcePlugin, Plugin: plugin, Model: asset.ID, Action: "absent"})
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("prune %s/%s: %w", plugin, asset.ID, err)
			}
			return output.Print(cmd, output.ModelActionOutput{Source: output.ModelSourcePlugin, Plugin: plugin, Model: asset.ID, Dir: dir, Action: "removed"})
		},
	}
}
