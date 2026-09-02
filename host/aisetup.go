package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/credentials"
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
	// config, when set — resolved through config.ResolveAIDefault, the one
	// resolver every surface reads.
	DefaultProvider string `json:"defaultProvider,omitempty"`
	DefaultModel    string `json:"defaultModel,omitempty"`
	// DefaultSource carries where the default came from, so a diagnostic can say
	// which file or env var to change instead of asserting that nothing is
	// configured.
	DefaultSource config.AIDefault `json:"defaultSource,omitzero"`
}

// Configured reports whether any AI provider is already usable without setup:
// a configured default, a saved credential, or an API key in the environment.
func (d AIDetection) Configured() bool {
	return d.DefaultProvider != "" || len(d.SavedCredentialProviders) > 0 || len(d.EnvKeyProviders) > 0
}

// StandingLine renders an accurate one-line account of what is already
// available, for the wizard header and for the "nothing configured" path — which
// must never claim nothing is configured when a default, a saved credential or
// an env key exists.
func (d AIDetection) StandingLine() string {
	var parts []string
	if d.DefaultProvider != "" {
		parts = append(parts, "default: "+d.DefaultSource.Describe())
	}
	if len(d.SavedCredentialProviders) > 0 {
		parts = append(parts, "saved credentials: "+strings.Join(d.SavedCredentialProviders, ", "))
	}
	if len(d.EnvKeyProviders) > 0 {
		parts = append(parts, "API keys in the environment: "+strings.Join(d.EnvKeyProviders, ", "))
	}
	if len(parts) == 0 {
		return "no AI provider configured"
	}
	return strings.Join(parts, " · ")
}

// aiDetectOllamaTimeout bounds the local Ollama liveness probe.
const aiDetectOllamaTimeout = 2 * time.Second

// DetectAIOptions gathers the machine's AI options. OllamaBaseURL "" probes
// the default localhost port. Safe with a nil credential store or config.
func (a *App) DetectAIOptions(ctx context.Context, ollamaBaseURLParam string) AIDetection {
	ctx = ctxOrBackground(ctx)
	var det AIDetection

	det.ClaudeCodePath, det.ClaudeCode = aiprovider.ClaudeCodeBinaryPath()

	probeCtx, cancel := context.WithTimeout(ctx, aiDetectOllamaTimeout)
	_, oerr := aiprovider.NewOllamaManager(ollamaBaseURLParam).Version(probeCtx)
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
	def := config.ResolveAIDefault(a.Config)
	det.DefaultProvider = def.Provider
	det.DefaultModel = def.Model
	det.DefaultSource = def
	return det
}

// ---------------------------------------------------------------------------
// Setup wizard
// ---------------------------------------------------------------------------

// AISetupChoice is one selectable option in the setup wizard.
type AISetupChoice struct {
	Provider string // provider id (ai.Provider value)
	model    string // default model written alongside
	label    string // one-line rendering
	needsKey bool   // prompt for an API key when chosen (no env key present)
}

// Label is the choice's one-line rendering, for a prompter to display.
func (c AISetupChoice) Label() string { return c.label }

// Model is the model written alongside the provider when this choice is taken.
func (c AISetupChoice) Model() string { return c.model }

// NeedsKey reports whether choosing this provider requires entering an API key
// (no conventional env var already carries one).
func (c AISetupChoice) NeedsKey() bool { return c.needsKey }

// AISetupPrompter renders the wizard's three questions. It exists so the
// terminal experience is a presentation concern: host owns detection, the
// ordered choices, the live check and persistence, and knows nothing about how
// the questions are drawn. The CLI supplies a form-based implementation
// (charmbracelet/huh — the repo's established interactive convention, already
// used by `kapi init`); host's own default is a plain numbered reader, which is
// what a bare terminal, a dumb TTY, and the tests use.
type AISetupPrompter interface {
	// SelectProvider asks for one of choices, defaulting to index def (0-based),
	// and returns the chosen index.
	SelectProvider(choices []AISetupChoice, def int) (int, error)
	// APIKey asks for a provider API key. Returning "" aborts with guidance.
	APIKey(providerLabel string) (string, error)
	// Confirm asks a yes/no question with the given default.
	Confirm(prompt string, def bool) (bool, error)
}

// AISetupIO carries the wizard's injectable dependencies so tests can script
// stdin, fake detection, and stub the live check and persistence.
type AISetupIO struct {
	In     io.Reader
	Out    io.Writer
	IsTTY  func() bool
	Detect func(ctx context.Context) AIDetection
	// liveCheck runs a tiny call through the chosen provider ("" apiKey for
	// keyless ones). Defaults to a real one-message Chat.
	LiveCheck func(ctx context.Context, provider, model, apiKey string) error
	// SetDefault persists the chosen provider and model. Defaults to
	// config.SetAIDefault — the one write path, which also clears a stored model
	// when the new provider brings none, so a model chosen for a previous provider
	// cannot survive as an orphan attached to a different one.
	SetDefault func(provider, model string) error
	// Prompter renders the questions. Defaults to the plain numbered reader over
	// In/Out; the CLI injects a form-based one (see App.AISetupPrompter).
	Prompter AISetupPrompter
}

func (a *App) DefaultAISetupIO(cmd Command) AISetupIO {
	if a.AISetupIOOverride != nil {
		return *a.AISetupIOOverride
	}
	io := AISetupIO{
		In:        cmd.InOrStdin(),
		Out:       cmd.ErrOrStderr(),
		IsTTY:     defaultIsStdinTTY,
		Detect:    func(ctx context.Context) AIDetection { return a.DetectAIOptions(ctx, "") },
		LiveCheck: aiSetupLiveCheck,
		SetDefault: func(provider, model string) error {
			return config.SetAIDefault(a.Config, provider, model)
		},
	}
	// A front-end that registered a richer prompter gets it for every wizard
	// entry point — `kapi models setup` and the inline one mid-`kapi up` alike,
	// so the two are never a different experience.
	if a.AISetupPrompter != nil {
		io.Prompter = a.AISetupPrompter
	}
	return io
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
	"no AI provider is configured and stdin is not a terminal. Configure one non-interactively:\n" +
		"  • set an API key env var (ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY), or\n" +
		"  • kapi config set ai.provider claude-code   # use the local Claude Code login\n" +
		"  • kapi config set ai.provider ollama        # local models via Ollama\n" +
		"  • kapi credentials add <name> --provider <id> --api-key <key>\n" +
		"then re-run; on a terminal, `kapi models setup` walks through this interactively")

// BuildAISetupChoices orders the selectable providers best-detected first:
// Claude Code when present, providers with an env key, Ollama when running,
// then the key-entry cloud providers, then the demo engine.
func BuildAISetupChoices(det AIDetection) []AISetupChoice {
	var choices []AISetupChoice
	added := map[string]bool{}
	add := func(c AISetupChoice) {
		if !added[c.Provider] {
			added[c.Provider] = true
			choices = append(choices, c)
		}
	}

	if det.ClaudeCode {
		add(AISetupChoice{
			Provider: string(aiprovider.ClaudeCode),
			model:    aiprovider.DefaultClaudeCodeModel,
			label:    "Claude Code (sonnet): detected · uses your Claude subscription, no API key",
		})
	}
	for _, p := range det.EnvKeyProviders {
		info, ok := aiprovider.ProviderInfoFor(aiprovider.ProviderID(p))
		if !ok {
			continue
		}
		add(AISetupChoice{
			Provider: p,
			model:    info.DefaultModel,
			label:    fmt.Sprintf("%s (%s): API key found in environment", info.Label, info.DefaultModel),
		})
	}
	if det.Ollama {
		add(AISetupChoice{
			Provider: string(aiprovider.Ollama),
			model:    aiprovider.DefaultOllamaModel,
			label:    fmt.Sprintf("Ollama (%s): detected · local models, content stays on this machine", aiprovider.DefaultOllamaModel),
		})
	}
	for _, id := range []aiprovider.ProviderID{aiprovider.Anthropic, aiprovider.OpenAI, aiprovider.Gemini} {
		info, _ := aiprovider.ProviderInfoFor(id)
		add(AISetupChoice{
			Provider: string(id),
			model:    info.DefaultModel,
			label:    fmt.Sprintf("%s (%s): paste an API key", info.Label, info.DefaultModel),
			needsKey: true,
		})
	}
	if !det.Ollama {
		add(AISetupChoice{
			Provider: string(aiprovider.Ollama),
			model:    aiprovider.DefaultOllamaModel,
			label:    "Ollama: local models (not running; start it with `kapi models ollama install`)",
		})
	}
	add(AISetupChoice{
		Provider: string(aiprovider.Demo),
		model:    "",
		label:    "Demo engine: deterministic illustrative output, no AI and no key",
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

// RunAISetupWizard renders detections, lets the user pick the default
// provider (one Confirm), stores a pasted key when needed, optionally runs a
// tiny live check, and persists ai.Provider/ai.model. compact drops the
// banner for inline (mid-command) use. Returns the chosen provider id.
func (a *App) RunAISetupWizard(ctx context.Context, io AISetupIO, compact bool) (string, error) {
	if !io.IsTTY() {
		return "", errAISetupNotInteractive
	}
	det := io.Detect(ctx)
	choices := BuildAISetupChoices(det)

	w := io.Out
	switch {
	case compact && det.Configured():
		// Something IS configured — the run needed setup for another reason
		// (an unusable default, a missing key). Saying "nothing is configured"
		// here sent people looking for a setting that was already there.
		fmt.Fprintf(w, "\nAn AI provider is needed to continue. Currently: %s.\n\n", det.StandingLine())
	case compact:
		fmt.Fprintf(w, "\nNo AI provider is configured yet. Pick one to continue (see `kapi models setup`).\n\n")
	default:
		fmt.Fprintf(w, "\nConnect an AI provider\n\n")
		if det.Configured() {
			fmt.Fprintf(w, "Currently: %s\n\n", det.StandingLine())
		}
	}
	if det.ClaudeCode || det.Ollama || len(det.EnvKeyProviders) > 0 {
		fmt.Fprintf(w, "Detected on this machine:\n")
		if det.ClaudeCode {
			fmt.Fprintf(w, "  ✓ Claude Code detected: uses your Claude subscription\n")
		}
		if det.Ollama {
			fmt.Fprintf(w, "  ✓ Ollama running: local models, no key needed\n")
		}
		for _, p := range det.EnvKeyProviders {
			fmt.Fprintf(w, "  ✓ %s API key set in environment\n", aiProviderDisplayLabel(p))
		}
		fmt.Fprintln(w)
	}

	prompter := io.Prompter
	if prompter == nil {
		prompter = newLinePrompter(io.In, w)
	}

	idx, err := prompter.SelectProvider(choices, 0)
	if err != nil {
		return "", err
	}
	if idx < 0 || idx >= len(choices) {
		return "", errors.New("no provider selected")
	}
	choice := choices[idx]

	// Key entry (stored through the existing credentials flow) when the chosen
	// provider needs one and the environment doesn't already carry it.
	apiKey := ""
	if choice.needsKey {
		entered, rerr := prompter.APIKey(aiProviderDisplayLabel(choice.Provider))
		if rerr != nil {
			return "", rerr
		}
		apiKey = strings.TrimSpace(entered)
		if apiKey == "" {
			return "", fmt.Errorf("no API key entered. Run `kapi models setup` again, or set %s_API_KEY", strings.ToUpper(choice.Provider))
		}
		if a.Credentials == nil {
			return "", errors.New("credential store unavailable")
		}
		cfg, uerr := a.Credentials.Upsert(credentials.ProviderConfig{
			Name:         choice.Provider,
			ProviderType: choice.Provider,
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
	if choice.Provider != string(aiprovider.Demo) {
		ok, cerr := prompter.Confirm("Run a quick test call to verify the setup?", true)
		if cerr != nil {
			return "", cerr
		}
		if ok {
			fmt.Fprintf(w, "… testing %s\n", aiProviderDisplayLabel(choice.Provider))
			if lerr := io.LiveCheck(ctx, choice.Provider, choice.model, apiKey); lerr != nil {
				return "", fmt.Errorf("test call failed: %w", lerr)
			}
			fmt.Fprintf(w, "✓ test call succeeded\n")
		}
	}

	// Persist through the one write path (config.SetAIDefault by default), which
	// clears a stored model when the chosen provider brings none: picking
	// claude-code after ollama/gemma must not leave `model: gemma…` attached to
	// it. Injectable so tests observe the write without touching a real config.
	setDefault := io.SetDefault
	if setDefault == nil {
		setDefault = func(provider, model string) error {
			return config.SetAIDefault(nil, provider, model)
		}
	}
	if err := setDefault(choice.Provider, choice.model); err != nil {
		return "", err
	}
	// Mirror into the in-memory config — separate from persistence, because this
	// is about the *running* process: an inline wizard mid-`kapi up` must take
	// effect without re-reading the file. The model is written even when empty, so
	// the mirror clears alongside the stored value.
	if a.Config != nil {
		a.Config.Set(config.KeyAIProvider, choice.Provider)
		a.Config.Set(config.KeyAIModel, choice.model)
	}

	ready := aiProviderDisplayLabel(choice.Provider)
	if choice.model != "" {
		fmt.Fprintf(w, "\n✓ kapi will translate with %s (%s). Try: kapi up\n", ready, choice.model)
	} else {
		fmt.Fprintf(w, "\n✓ kapi will translate with %s. Try: kapi up\n", ready)
	}
	return choice.Provider, nil
}

// EnsureAIProviderInteractive runs the compact setup wizard when a command is
// about to need an AI provider and none is configured anywhere (no ai.Provider
// default, no saved credential, no *_API_KEY). On a TTY the wizard runs inline
// and the command continues; off a TTY behavior is unchanged (the existing
// keys-only error paths fire downstream). Explicit --provider/--credential/
// --api-key flags always win and skip the wizard.
func (a *App) EnsureAIProviderInteractive(cmd Command) error {
	for _, flag := range []string{"provider", "credential", "api-key"} {
		if f := cmd.Flags().Lookup(flag); f != nil && cmd.Flags().Changed(flag) {
			return nil
		}
	}
	io := a.DefaultAISetupIO(cmd)
	if !io.IsTTY() {
		return nil
	}
	det := io.Detect(cmd.Context())
	if det.Configured() {
		return nil
	}
	_, err := a.RunAISetupWizard(cmd.Context(), io, true)
	return err
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

// promptYesNo asks a yes/no question, using def for empty input.
func promptYesNo(r *promptReader, w io.Writer, prompt string, def bool) (bool, error) {
	fmt.Fprint(w, prompt)
	line, err := r.line()
	if err != nil {
		return false, fmt.Errorf("read answer: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def, nil
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// linePrompter is host's own AISetupPrompter: a numbered list read
// line-at-a-time. It is the default because it is the implementation that
// always works — a dumb terminal, a pipe with scripted answers, the tests — and
// because host must not depend on a TUI (the desktop links host too). A
// front-end that can do better registers its own (see App.AISetupPrompter).
type linePrompter struct {
	r *promptReader
	w io.Writer
}

func newLinePrompter(r io.Reader, w io.Writer) *linePrompter {
	return &linePrompter{r: newLineReader(r), w: w}
}

func (p *linePrompter) SelectProvider(choices []AISetupChoice, def int) (int, error) {
	fmt.Fprintf(p.w, "Choose the default AI provider:\n")
	for i, c := range choices {
		fmt.Fprintf(p.w, "  %d) %s\n", i+1, c.label)
	}
	idx, err := promptSelect(p.r, p.w, "Select", len(choices), def+1)
	if err != nil {
		return 0, err
	}
	return idx - 1, nil
}

func (p *linePrompter) APIKey(providerLabel string) (string, error) {
	fmt.Fprintf(p.w, "Paste your %s API key (input is not masked): ", providerLabel)
	return p.r.line()
}

func (p *linePrompter) Confirm(prompt string, def bool) (bool, error) {
	suffix := " [y/N] "
	if def {
		suffix = " [Y/n] "
	}
	return promptYesNo(p.r, p.w, prompt+suffix, def)
}
