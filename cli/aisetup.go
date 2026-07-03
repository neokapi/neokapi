package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli/config"
	"github.com/neokapi/neokapi/cli/credentials"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// AIDetection reports the AI options available on this machine, gathered
// without any credential prompts: which keyless providers are present, which
// conventional *_API_KEY environment variables are set, what credentials are
// already saved, and the configured default. Shared by `kapi models setup`,
// the first-run inline wizard, and the desktop's DetectAIProviders binding.
type AIDetection struct {
	// ClaudeCode is true when the Claude Code CLI (`claude`) is on PATH. Auth
	// is verified lazily at first call, not here, so detection stays instant.
	ClaudeCode bool `json:"claudeCode"`
	// ClaudeCodePath is the resolved binary path when detected.
	ClaudeCodePath string `json:"claudeCodePath,omitempty"`
	// Ollama is true when a local Ollama server responds on the default port.
	Ollama bool `json:"ollama"`
	// EnvKeyProviders are provider ids whose conventional API-key env var is
	// set (e.g. ANTHROPIC_API_KEY → anthropic), sorted.
	EnvKeyProviders []string `json:"envKeyProviders,omitempty"`
	// SavedCredentialProviders are provider ids with a saved credential in the
	// store (deduplicated, in store order).
	SavedCredentialProviders []string `json:"savedCredentialProviders,omitempty"`
	// DefaultProvider / DefaultModel are the shared ai.provider / ai.model app
	// config, when set.
	DefaultProvider string `json:"defaultProvider,omitempty"`
	DefaultModel    string `json:"defaultModel,omitempty"`
}

// Configured reports whether any AI provider is already usable without setup:
// a configured default, a saved credential, or an API key in the environment.
func (d AIDetection) Configured() bool {
	return d.DefaultProvider != "" || len(d.SavedCredentialProviders) > 0 || len(d.EnvKeyProviders) > 0
}

// aiDetectOllamaTimeout bounds the local Ollama liveness probe.
const aiDetectOllamaTimeout = 2 * time.Second

// DetectAIOptions gathers the machine's AI options. ollamaBaseURL "" probes
// the default localhost port. Safe with a nil credential store or config.
func (a *App) DetectAIOptions(ctx context.Context, ollamaBaseURL string) AIDetection {
	if ctx == nil {
		ctx = context.Background()
	}
	var det AIDetection

	det.ClaudeCodePath, det.ClaudeCode = aiprovider.ClaudeCodeBinaryPath()

	probeCtx, cancel := context.WithTimeout(ctx, aiDetectOllamaTimeout)
	_, oerr := aiprovider.NewOllamaManager(ollamaBaseURL).Version(probeCtx)
	cancel()
	det.Ollama = oerr == nil

	det.EnvKeyProviders = credentials.ProvidersWithEnvKey()

	if a.Credentials != nil {
		seen := map[string]bool{}
		for _, c := range a.Credentials.List() {
			if !seen[c.ProviderType] {
				seen[c.ProviderType] = true
				det.SavedCredentialProviders = append(det.SavedCredentialProviders, c.ProviderType)
			}
		}
	}
	if a.Config != nil {
		det.DefaultProvider = a.Config.GetString(config.KeyAIProvider)
		det.DefaultModel = a.Config.GetString(config.KeyAIModel)
	}
	return det
}

// ---------------------------------------------------------------------------
// Setup wizard
// ---------------------------------------------------------------------------

// aiSetupChoice is one selectable option in the setup wizard.
type aiSetupChoice struct {
	provider string // provider id (ai.provider value)
	model    string // default model written alongside
	label    string // one-line rendering
	needsKey bool   // prompt for an API key when chosen (no env key present)
}

// aiSetupIO carries the wizard's injectable dependencies so tests can script
// stdin, fake detection, and stub the live check and persistence.
type aiSetupIO struct {
	in     io.Reader
	out    io.Writer
	isTTY  func() bool
	detect func(ctx context.Context) AIDetection
	// liveCheck runs a tiny call through the chosen provider ("" apiKey for
	// keyless ones). Defaults to a real one-message Chat.
	liveCheck func(ctx context.Context, provider, model, apiKey string) error
	// setConfig persists one global config key. Defaults to config.SetGlobalConfig.
	setConfig func(key, value string) error
}

func (a *App) defaultAISetupIO(cmd *cobra.Command) aiSetupIO {
	if a.aiSetupIOOverride != nil {
		return *a.aiSetupIOOverride
	}
	return aiSetupIO{
		in:        cmd.InOrStdin(),
		out:       cmd.ErrOrStderr(),
		isTTY:     defaultIsStdinTTY,
		detect:    func(ctx context.Context) AIDetection { return a.DetectAIOptions(ctx, "") },
		liveCheck: aiSetupLiveCheck,
		setConfig: func(key, value string) error { return config.SetGlobalConfig(key, value) },
	}
}

// aiSetupLiveCheck sends one tiny prompt through the chosen provider to prove
// the path works end-to-end (claude-code auth, ollama model presence, key
// validity). Bounded so setup never hangs.
func aiSetupLiveCheck(ctx context.Context, provider, model, apiKey string) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	p, err := aiprovider.NewProvider(aiprovider.ProviderID(provider), aiprovider.Config{Model: model, APIKey: apiKey})
	if err != nil {
		return err
	}
	defer p.Close()
	_, err = p.Chat(ctx, []aiprovider.Message{aiprovider.TextMessage("user", "Reply with exactly: OK")})
	return err
}

// errAISetupNotInteractive is returned when setup is required but stdin is not
// a terminal; the message is the complete non-TTY guidance.
var errAISetupNotInteractive = errors.New(
	"no AI provider is configured and stdin is not a terminal — configure one non-interactively:\n" +
		"  • set an API key env var (ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY), or\n" +
		"  • kapi config set ai.provider claude-code   # use the local Claude Code login\n" +
		"  • kapi config set ai.provider ollama        # local models via Ollama\n" +
		"  • kapi credentials add <name> --provider <id> --api-key <key>\n" +
		"then re-run; on a terminal, `kapi models setup` walks through this interactively")

// buildAISetupChoices orders the selectable providers best-detected first:
// Claude Code when present, providers with an env key, Ollama when running,
// then the key-entry cloud providers, then the demo engine.
func buildAISetupChoices(det AIDetection) []aiSetupChoice {
	var choices []aiSetupChoice
	added := map[string]bool{}
	add := func(c aiSetupChoice) {
		if !added[c.provider] {
			added[c.provider] = true
			choices = append(choices, c)
		}
	}

	if det.ClaudeCode {
		add(aiSetupChoice{
			provider: string(aiprovider.ClaudeCode),
			model:    aiprovider.DefaultClaudeCodeModel,
			label:    "Claude Code (sonnet) — detected · uses your Claude subscription, no API key",
		})
	}
	for _, p := range det.EnvKeyProviders {
		info, ok := aiprovider.ProviderInfoFor(aiprovider.ProviderID(p))
		if !ok {
			continue
		}
		add(aiSetupChoice{
			provider: p,
			model:    info.DefaultModel,
			label:    fmt.Sprintf("%s (%s) — API key found in environment", info.Label, info.DefaultModel),
		})
	}
	if det.Ollama {
		add(aiSetupChoice{
			provider: string(aiprovider.Ollama),
			model:    aiprovider.DefaultOllamaModel,
			label:    fmt.Sprintf("Ollama (%s) — detected · local models, content stays on this machine", aiprovider.DefaultOllamaModel),
		})
	}
	for _, id := range []aiprovider.ProviderID{aiprovider.Anthropic, aiprovider.OpenAI, aiprovider.Gemini} {
		info, _ := aiprovider.ProviderInfoFor(id)
		add(aiSetupChoice{
			provider: string(id),
			model:    info.DefaultModel,
			label:    fmt.Sprintf("%s (%s) — paste an API key", info.Label, info.DefaultModel),
			needsKey: true,
		})
	}
	if !det.Ollama {
		add(aiSetupChoice{
			provider: string(aiprovider.Ollama),
			model:    aiprovider.DefaultOllamaModel,
			label:    "Ollama — local models (not running; start it with `kapi models ollama install`)",
		})
	}
	add(aiSetupChoice{
		provider: string(aiprovider.Demo),
		model:    "",
		label:    "Demo engine — deterministic illustrative output, no AI and no key",
	})
	return choices
}

// aiProviderDisplayLabel renders a provider id for the ready-to-go line.
func aiProviderDisplayLabel(provider string) string {
	if info, ok := aiprovider.ProviderInfoFor(aiprovider.ProviderID(provider)); ok {
		return info.Label
	}
	return provider
}

// runAISetupWizard renders detections, lets the user pick the default
// provider (one confirm), stores a pasted key when needed, optionally runs a
// tiny live check, and persists ai.provider/ai.model. compact drops the
// banner for inline (mid-command) use. Returns the chosen provider id.
func (a *App) runAISetupWizard(ctx context.Context, io aiSetupIO, compact bool) (string, error) {
	if !io.isTTY() {
		return "", errAISetupNotInteractive
	}
	det := io.detect(ctx)
	choices := buildAISetupChoices(det)

	w := io.out
	if compact {
		fmt.Fprintf(w, "\nNo AI provider is configured yet — pick one to continue (see `kapi models setup`).\n\n")
	} else {
		fmt.Fprintf(w, "\nConnect an AI provider\n\n")
	}
	if det.ClaudeCode || det.Ollama || len(det.EnvKeyProviders) > 0 {
		fmt.Fprintf(w, "Detected on this machine:\n")
		if det.ClaudeCode {
			fmt.Fprintf(w, "  ✓ Claude Code detected — uses your Claude subscription\n")
		}
		if det.Ollama {
			fmt.Fprintf(w, "  ✓ Ollama running — local models, no key needed\n")
		}
		for _, p := range det.EnvKeyProviders {
			fmt.Fprintf(w, "  ✓ %s API key set in environment\n", aiProviderDisplayLabel(p))
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Choose the default AI provider:\n")
	for i, c := range choices {
		fmt.Fprintf(w, "  %d) %s\n", i+1, c.label)
	}
	reader := newLineReader(io.in)
	idx, err := promptSelect(reader, w, "Select", len(choices), 1)
	if err != nil {
		return "", err
	}
	choice := choices[idx-1]

	// Key entry (stored through the existing credentials flow) when the chosen
	// provider needs one and the environment doesn't already carry it.
	apiKey := ""
	if choice.needsKey {
		fmt.Fprintf(w, "Paste your %s API key (input is not masked): ", aiProviderDisplayLabel(choice.provider))
		line, rerr := reader.line()
		if rerr != nil {
			return "", rerr
		}
		apiKey = strings.TrimSpace(line)
		if apiKey == "" {
			return "", fmt.Errorf("no API key entered — run `kapi models setup` again, or set %s_API_KEY", strings.ToUpper(choice.provider))
		}
		if a.Credentials == nil {
			return "", errors.New("credential store unavailable")
		}
		cfg, uerr := a.Credentials.Upsert(credentials.ProviderConfig{
			Name:         choice.provider,
			ProviderType: choice.provider,
			Model:        choice.model,
		})
		if uerr != nil {
			return "", fmt.Errorf("save provider config: %w", uerr)
		}
		if kerr := a.Credentials.SetAPIKey(cfg.ID, apiKey); kerr != nil {
			return "", fmt.Errorf("store API key in keychain: %w", kerr)
		}
		fmt.Fprintf(w, "✓ key saved to the OS keychain as credential %q\n", cfg.Name)
	}

	// Tiny live check through the chosen provider — skippable, and pointless
	// for the offline demo stub.
	if choice.provider != string(aiprovider.Demo) {
		ok, cerr := promptYesNo(reader, w, "Run a quick test call to verify the setup? [Y/n] ")
		if cerr != nil {
			return "", cerr
		}
		if ok {
			fmt.Fprintf(w, "… testing %s\n", aiProviderDisplayLabel(choice.provider))
			if lerr := io.liveCheck(ctx, choice.provider, choice.model, apiKey); lerr != nil {
				return "", fmt.Errorf("test call failed: %w", lerr)
			}
			fmt.Fprintf(w, "✓ test call succeeded\n")
		}
	}

	if err := io.setConfig(config.KeyAIProvider, choice.provider); err != nil {
		return "", fmt.Errorf("set %s: %w", config.KeyAIProvider, err)
	}
	if choice.model != "" {
		if err := io.setConfig(config.KeyAIModel, choice.model); err != nil {
			return "", fmt.Errorf("set %s: %w", config.KeyAIModel, err)
		}
	}
	// Mirror into the in-memory config so the current process (an inline
	// wizard mid-`kapi up`) picks the choice up without reloading.
	if a.Config != nil {
		a.Config.Set(config.KeyAIProvider, choice.provider)
		if choice.model != "" {
			a.Config.Set(config.KeyAIModel, choice.model)
		}
	}

	ready := aiProviderDisplayLabel(choice.provider)
	if choice.model != "" {
		fmt.Fprintf(w, "\n✓ kapi will translate with %s (%s). Try: kapi up\n", ready, choice.model)
	} else {
		fmt.Fprintf(w, "\n✓ kapi will translate with %s. Try: kapi up\n", ready)
	}
	return choice.provider, nil
}

// EnsureAIProviderInteractive runs the compact setup wizard when a command is
// about to need an AI provider and none is configured anywhere (no ai.provider
// default, no saved credential, no *_API_KEY). On a TTY the wizard runs inline
// and the command continues; off a TTY behavior is unchanged (the existing
// keys-only error paths fire downstream). Explicit --provider/--credential/
// --api-key flags always win and skip the wizard.
func (a *App) EnsureAIProviderInteractive(cmd *cobra.Command) error {
	for _, flag := range []string{"provider", "credential", "api-key"} {
		if f := cmd.Flags().Lookup(flag); f != nil && cmd.Flags().Changed(flag) {
			return nil
		}
	}
	io := a.defaultAISetupIO(cmd)
	if !io.isTTY() {
		return nil
	}
	det := io.detect(cmd.Context())
	if det.Configured() {
		return nil
	}
	_, err := a.runAISetupWizard(cmd.Context(), io, true)
	return err
}

// newModelsSetupCmd builds `kapi models setup` — the interactive first-run
// provider wizard.
func (a *App) newModelsSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactively pick and verify the default AI provider",
		Long: "Detects the AI options on this machine — the Claude Code CLI (uses your\n" +
			"Claude subscription, no API key), a running Ollama server (local models),\n" +
			"and API keys already present in the environment — then walks through picking\n" +
			"the default provider, storing an API key when one is needed, and verifying\n" +
			"the choice with a tiny test call. Writes ai.provider / ai.model to the global\n" +
			"config (shared with Kapi Desktop).\n\n" +
			"Requires a terminal; in CI set an API key env var or use `kapi config set`.",
		Example: "  kapi models setup",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := a.runAISetupWizard(cmd.Context(), a.defaultAISetupIO(cmd), false)
			return err
		},
	}
	return cmd
}

// ---------------------------------------------------------------------------
// Minimal prompt helpers (bufio-backed, mirroring cli/projectreq.go's style)
// ---------------------------------------------------------------------------

// newLineReader wraps an io.Reader for line-at-a-time prompts. A bufio.Reader
// would over-read from shared stdin; this reads byte-at-a-time so an inline
// wizard leaves the rest of stdin untouched for the continuing command.
func newLineReader(r io.Reader) *promptReader { return &promptReader{r: r} }

type promptReader struct {
	r    io.Reader
	rest []byte
}

// line reads up to the next newline (or EOF) and returns the line without it.
func (p *promptReader) line() (string, error) {
	var out []byte
	buf := make([]byte, 1)
	for {
		if len(p.rest) > 0 {
			b := p.rest[0]
			p.rest = p.rest[1:]
			if b == '\n' {
				return strings.TrimRight(string(out), "\r"), nil
			}
			out = append(out, b)
			continue
		}
		n, err := p.r.Read(buf)
		if n > 0 {
			p.rest = append(p.rest, buf[:n]...)
			continue
		}
		if err != nil {
			if len(out) > 0 {
				return strings.TrimRight(string(out), "\r"), nil
			}
			return "", err
		}
	}
}

// promptSelect asks for a 1-based selection, defaulting on empty input.
func promptSelect(r *promptReader, w io.Writer, label string, n, def int) (int, error) {
	for {
		fmt.Fprintf(w, "%s [%d]: ", label, def)
		line, err := r.line()
		if err != nil {
			return 0, fmt.Errorf("read selection: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def, nil
		}
		v, cerr := strconv.Atoi(line)
		if cerr == nil && v >= 1 && v <= n {
			return v, nil
		}
		fmt.Fprintf(w, "Enter a number between 1 and %d.\n", n)
	}
}

// promptYesNo asks a yes/no question, defaulting to yes.
func promptYesNo(r *promptReader, w io.Writer, prompt string) (bool, error) {
	fmt.Fprint(w, prompt)
	line, err := r.line()
	if err != nil {
		return false, fmt.Errorf("read answer: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "", "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
