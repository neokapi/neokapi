package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DefaultClaudeCodeModel is the model alias a fresh claude-code provider uses.
// Aliases (sonnet, opus, haiku) and full model ids both pass straight through
// to the Claude Code binary, which resolves them against the account's access.
const DefaultClaudeCodeModel = "sonnet"

// claudeCodeDefaultTimeout bounds a single Claude Code CLI invocation. The
// binary spins up a full agent session per call, so the default is generous;
// override with KAPI_CLAUDE_CODE_TIMEOUT (a Go duration, e.g. "3m").
const claudeCodeDefaultTimeout = 5 * time.Minute

// ClaudeCodeProvider implements LLMProvider by shelling out to the locally
// installed Claude Code CLI (`claude`), so translation runs on the user's
// Claude subscription instead of a metered API key.
//
// Verified contract (claude 2.1.x):
//
//	claude -p --output-format json --model <model> --tools "" \
//	       --no-session-persistence --setting-sources "" [--json-schema <schema>]
//
// The prompt is written to stdin. The binary prints a single JSON envelope:
// {"type":"result","subtype":"success","is_error":false,"result":"...",
// "usage":{"input_tokens":N,"output_tokens":N,...},"num_turns":N,...}.
// API failures keep exit code 0 but set is_error=true plus api_error_status
// and put the human-readable message in "result". With --json-schema the
// schema-conforming object arrives in "structured_output". Tool use is fully
// disabled via --tools "" which also bounds the call to plain text turns.
type ClaudeCodeProvider struct {
	config Config
	// binary is the executable to invoke. Defaults to "claude" (resolved via
	// PATH); KAPI_CLAUDE_CODE_BIN overrides it (also used by tests to point at
	// a shim).
	binary string
	// timeout bounds one CLI invocation.
	timeout time.Duration
}

// NewClaudeCodeProvider creates a Claude Code CLI provider. No API key is
// required — authentication is the Claude Code login on this machine, verified
// lazily on the first call (mapped to ClaudeCodeError when missing).
func NewClaudeCodeProvider(cfg Config) *ClaudeCodeProvider {
	if cfg.Model == "" {
		cfg.Model = DefaultClaudeCodeModel
	}
	p := &ClaudeCodeProvider{config: cfg, binary: "claude", timeout: claudeCodeDefaultTimeout}
	if bin := os.Getenv("KAPI_CLAUDE_CODE_BIN"); bin != "" {
		p.binary = bin
	}
	if raw := os.Getenv("KAPI_CLAUDE_CODE_TIMEOUT"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			p.timeout = d
		}
	}
	return p
}

func (p *ClaudeCodeProvider) Name() ProviderID { return ClaudeCode }

// InputModalities — the CLI bridge is text-only: media would have to travel as
// files on disk, which the provider deliberately does not do.
func (p *ClaudeCodeProvider) InputModalities() []Modality { return nil }

func (p *ClaudeCodeProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	return StandardTranslate(ctx, p.Name(), p.Chat, req, 0.85)
}

func (p *ClaudeCodeProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	return p.invoke(ctx, messages, nil)
}

// ChatStructured uses the CLI's native --json-schema flag; the conforming
// object is returned in the envelope's structured_output field.
func (p *ClaudeCodeProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	return p.invoke(ctx, messages, &schema)
}

func (p *ClaudeCodeProvider) Close() error { return nil }

// ClaudeCodeBinaryPath resolves the Claude Code executable, honouring
// KAPI_CLAUDE_CODE_BIN. ok is false when the binary is not installed.
func ClaudeCodeBinaryPath() (path string, ok bool) {
	bin := os.Getenv("KAPI_CLAUDE_CODE_BIN")
	if bin == "" {
		bin = "claude"
	}
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return "", false
	}
	return resolved, true
}

// ClaudeCodeDetected reports whether the Claude Code CLI is installed on this
// machine (PATH presence only — auth is verified lazily on first call, so
// detection stays instant and offline).
func ClaudeCodeDetected() bool {
	_, ok := ClaudeCodeBinaryPath()
	return ok
}

// ---------------------------------------------------------------------------
// Invocation
// ---------------------------------------------------------------------------

// claudeCodeEnvelope is the single-result JSON document `claude -p
// --output-format json` prints (fields verified against claude 2.1.181).
type claudeCodeEnvelope struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	IsError          bool            `json:"is_error"`
	APIErrorStatus   int             `json:"api_error_status"` // null → 0
	Result           string          `json:"result"`
	NumTurns         int             `json:"num_turns"`
	TotalCostUSD     float64         `json:"total_cost_usd"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	Usage            struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	// ModelUsage keys are the fully-resolved model ids (aliases like "sonnet"
	// come back as e.g. "claude-sonnet-4-5-...").
	ModelUsage map[string]json.RawMessage `json:"modelUsage"`
}

func (p *ClaudeCodeProvider) invoke(ctx context.Context, messages []Message, schema *JSONSchema) (*ChatResponse, error) {
	bin, err := exec.LookPath(p.binary)
	if err != nil {
		return nil, &ClaudeCodeError{Kind: ClaudeCodeErrNotInstalled}
	}

	systemPrompt, prompt := flattenClaudeCodeMessages(messages)
	args := []string{
		"-p",
		"--output-format", "json",
		"--model", p.config.Model,
		// Disable every tool: locks the call to pure text generation and
		// bounds it to plain model turns (no tool-use loop).
		"--tools", "",
		"--no-session-persistence",
		// Never load user/project settings: hooks, MCP servers, or CLAUDE.md
		// context must not leak into translation calls.
		"--setting-sources", "",
	}
	if systemPrompt != "" {
		// The system prompt is document-derived — translation instructions,
		// voice profile, terms, and surrounding content. On argv it is readable
		// by every process on the machine for the lifetime of the call, which
		// is why the user prompt already goes over stdin. Hand it over as a
		// file instead.
		//
		// --append-system-prompt-file is the faithful counterpart of the inline
		// flag, not an inline system turn: the CLI rejects the two together
		// ("Cannot use both --append-system-prompt and
		// --append-system-prompt-file"), so it treats them as one input in two
		// encodings. It is absent from `claude --help`'s option list — only
		// named under --bare — so if a future CLI drops it, this call fails
		// loudly with "unknown option" and the fix is to inline the prompt
		// again here.
		promptFile, cleanup, ferr := writeSystemPromptFile(systemPrompt)
		if ferr != nil {
			return nil, ferr
		}
		// Held until invoke returns, which is after both runWithRetry attempts
		// — they share this args slice.
		defer cleanup()
		args = append(args, "--append-system-prompt-file", promptFile)
	}
	if schema != nil {
		schemaJSON, err := json.Marshal(schema.Schema)
		if err != nil {
			return nil, fmt.Errorf("claude-code: marshal schema: %w", err)
		}
		args = append(args, "--json-schema", string(schemaJSON))
	}

	env, err := p.runWithRetry(ctx, bin, args, prompt)
	if err != nil {
		return nil, err
	}

	content := env.Result
	if schema != nil && len(env.StructuredOutput) > 0 && string(env.StructuredOutput) != "null" {
		content = string(env.StructuredOutput)
	}

	model := p.config.Model
	for id := range env.ModelUsage {
		model = id
		break
	}

	return &ChatResponse{
		Content: content,
		Model:   model,
		Usage: TokenUsage{
			InputTokens:         env.Usage.InputTokens,
			OutputTokens:        env.Usage.OutputTokens,
			CacheCreationTokens: env.Usage.CacheCreationInputTokens,
			CacheReadTokens:     env.Usage.CacheReadInputTokens,
		},
	}, nil
}

// writeSystemPromptFile stores the system prompt where only this user can read
// it, and returns the path plus a cleanup the caller must defer.
//
// os.CreateTemp opens at 0600, so the prompt is unreadable to other local
// accounts even though the containing temp directory is world-traversable.
func writeSystemPromptFile(systemPrompt string) (string, func(), error) {
	f, err := os.CreateTemp("", "kapi-claude-system-*.txt")
	if err != nil {
		return "", nil, fmt.Errorf("claude-code: create system prompt file: %w", err)
	}
	name := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(name)
	}
	if _, err := f.WriteString(systemPrompt); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("claude-code: write system prompt file: %w", err)
	}
	// Closed before the CLI reads it; cleanup's second Close is a harmless
	// no-op error we discard.
	if err := f.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("claude-code: write system prompt file: %w", err)
	}
	return name, cleanup, nil
}

// runWithRetry executes the CLI once, retrying a single time on transient
// failures (crash without a JSON envelope, 5xx upstream, overloaded). Auth,
// subscription-limit, and timeout failures never retry.
func (p *ClaudeCodeProvider) runWithRetry(ctx context.Context, bin string, args []string, prompt string) (*claudeCodeEnvelope, error) {
	env, err := p.runOnce(ctx, bin, args, prompt)
	if err == nil {
		return env, nil
	}
	if !claudeCodeTransient(err) || ctx.Err() != nil {
		return nil, err
	}
	env, retryErr := p.runOnce(ctx, bin, args, prompt)
	if retryErr != nil {
		return nil, retryErr
	}
	return env, nil
}

func (p *ClaudeCodeProvider) runOnce(ctx context.Context, bin string, args []string, prompt string) (*claudeCodeEnvelope, error) {
	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// On timeout CommandContext kills the binary, but Wait would still block on
	// the stdout/stderr pipes if a child it spawned inherited them (shell
	// fork-vs-exec differs across platforms). WaitDelay forces Run to return
	// shortly after the context ends regardless.
	cmd.WaitDelay = 2 * time.Second

	runErr := cmd.Run()

	if runCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		return nil, &ClaudeCodeError{Kind: ClaudeCodeErrTimeout, Message: p.timeout.String()}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	var env claudeCodeEnvelope
	if jsonErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &env); jsonErr != nil || env.Type == "" {
		// No parseable envelope: classify from stderr/exit — the CLI exits
		// non-zero with plain text for pre-flight failures (bad flags, no
		// login in some paths, crashes).
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if mapped := classifyClaudeCodeMessage(0, detail); mapped != nil {
			return nil, mapped
		}
		if runErr != nil {
			return nil, &ClaudeCodeError{Kind: ClaudeCodeErrCrashed, Message: firstClaudeCodeLine(detail, runErr.Error())}
		}
		return nil, &ClaudeCodeError{Kind: ClaudeCodeErrCrashed, Message: "unexpected output (no JSON result envelope)"}
	}

	if env.IsError {
		if mapped := classifyClaudeCodeMessage(env.APIErrorStatus, env.Result); mapped != nil {
			return nil, mapped
		}
		return nil, &ClaudeCodeError{Kind: ClaudeCodeErrOther, Message: firstClaudeCodeLine(env.Result, "unknown error"), Status: env.APIErrorStatus}
	}
	return &env, nil
}

// flattenClaudeCodeMessages folds a chat transcript into the CLI's single
// prompt + optional system prompt. System messages join into the system
// prompt; a lone user message passes through verbatim (the common translate
// shape); multi-turn transcripts are rendered with role labels.
func flattenClaudeCodeMessages(messages []Message) (systemPrompt, prompt string) {
	var system []string
	var rest []Message
	for _, m := range messages {
		if m.Role == "system" {
			system = append(system, m.Text())
			continue
		}
		rest = append(rest, m)
	}
	systemPrompt = strings.Join(system, "\n\n")

	if len(rest) == 1 {
		return systemPrompt, rest[0].Text()
	}
	var b strings.Builder
	for _, m := range rest {
		label := "User"
		if m.Role == "assistant" {
			label = "Assistant"
		}
		fmt.Fprintf(&b, "%s: %s\n\n", label, m.Text())
	}
	return systemPrompt, strings.TrimSuffix(b.String(), "\n")
}

// ---------------------------------------------------------------------------
// Error mapping
// ---------------------------------------------------------------------------

// ClaudeCodeErrorKind classifies Claude Code CLI failures.
type ClaudeCodeErrorKind int

const (
	// ClaudeCodeErrNotInstalled — the `claude` binary is not on PATH.
	ClaudeCodeErrNotInstalled ClaudeCodeErrorKind = iota
	// ClaudeCodeErrAuth — the binary is installed but not signed in.
	ClaudeCodeErrAuth
	// ClaudeCodeErrLimit — the Claude subscription usage window is exhausted.
	ClaudeCodeErrLimit
	// ClaudeCodeErrTimeout — one invocation exceeded the configured timeout.
	ClaudeCodeErrTimeout
	// ClaudeCodeErrCrashed — the binary exited without a JSON result envelope.
	ClaudeCodeErrCrashed
	// ClaudeCodeErrOther — an API-level error that is none of the above.
	ClaudeCodeErrOther
)

// ClaudeCodeError is a typed failure from the Claude Code CLI provider, with
// actionable, kind-specific rendering (never a silent stall).
type ClaudeCodeError struct {
	Kind ClaudeCodeErrorKind
	// Message carries kind-specific detail: the raw binary message for
	// Other/Crashed, the timeout for Timeout.
	Message string
	// ResetsAt is the human-readable resume hint for Limit errors when the
	// binary's message included a parseable time ("" otherwise).
	ResetsAt string
	// Status is the upstream API status for Other errors (0 when unknown).
	Status int
}

func (e *ClaudeCodeError) Error() string {
	switch e.Kind {
	case ClaudeCodeErrNotInstalled:
		return "claude-code: the `claude` binary was not found on PATH. Install Claude Code (https://claude.com/claude-code) or pick another provider with `kapi models setup`"
	case ClaudeCodeErrAuth:
		return "Claude Code isn't signed in. Run `claude` once to log in."
	case ClaudeCodeErrLimit:
		if e.ResetsAt != "" {
			return fmt.Sprintf("Claude subscription limit reached, resuming ~%s. Configure an API key (kapi models setup) for uninterrupted runs.", e.ResetsAt)
		}
		return "Claude subscription limit reached. Configure an API key (kapi models setup) for uninterrupted runs."
	case ClaudeCodeErrTimeout:
		return "claude-code: call timed out after " + e.Message + ". Set KAPI_CLAUDE_CODE_TIMEOUT to allow more time"
	case ClaudeCodeErrCrashed:
		return "claude-code: the `claude` binary failed: " + e.Message
	default:
		if e.Status != 0 {
			return fmt.Sprintf("claude-code: API error %d: %s", e.Status, e.Message)
		}
		return "claude-code: " + e.Message
	}
}

// classifyClaudeCodeMessage maps a binary/API failure message onto a typed
// error, matching the phrasings the CLI is known to emit (plus HTTP status
// hints). Returns nil when no confident classification is possible.
func classifyClaudeCodeMessage(status int, msg string) *ClaudeCodeError {
	l := strings.ToLower(msg)
	switch {
	case status == 429 ||
		strings.Contains(l, "usage limit reached") ||
		strings.Contains(l, "usage limit") && strings.Contains(l, "reached") ||
		strings.Contains(l, "limit will reset") ||
		strings.Contains(l, "hit your limit") ||
		strings.Contains(l, "out of extra usage") ||
		strings.Contains(l, "rate limit"):
		return &ClaudeCodeError{Kind: ClaudeCodeErrLimit, Message: msg, ResetsAt: parseClaudeCodeReset(msg)}
	case status == 401 || status == 403 ||
		strings.Contains(l, "not logged in") ||
		strings.Contains(l, "please run /login") ||
		strings.Contains(l, "please log in") ||
		strings.Contains(l, "run claude login") ||
		strings.Contains(l, "oauth token has expired") ||
		strings.Contains(l, "authentication_error") ||
		strings.Contains(l, "invalid api key"):
		return &ClaudeCodeError{Kind: ClaudeCodeErrAuth, Message: msg}
	}
	return nil
}

// claudeCodeResetUnix matches the "…limit reached|<unix-seconds>" suffix the
// API attaches to subscription-limit errors.
var claudeCodeResetUnix = regexp.MustCompile(`\|(\d{9,11})\s*$`)

// claudeCodeResetPhrase matches human phrasings like "resets 3am", "resets at
// 10:30pm", "will reset at 6pm (Europe/Oslo)".
var claudeCodeResetPhrase = regexp.MustCompile(`(?i)resets?\s+(?:at\s+)?(\d{1,2}(?::\d{2})?\s*(?:am|pm)?(?:\s*\([^)]+\))?)`)

// parseClaudeCodeReset best-effort extracts a human-readable resume time from
// a subscription-limit message. Returns "" when nothing parseable is present.
func parseClaudeCodeReset(msg string) string {
	if m := claudeCodeResetUnix.FindStringSubmatch(strings.TrimSpace(msg)); m != nil {
		if secs, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return time.Unix(secs, 0).Local().Format("3:04pm Mon Jan 2")
		}
	}
	if m := claudeCodeResetPhrase.FindStringSubmatch(msg); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// claudeCodeTransient reports whether a failure is worth one retry: upstream
// 5xx/overload or a crash without an envelope. Auth, limit, and timeout are
// deterministic and never retried.
func claudeCodeTransient(err error) bool {
	var ce *ClaudeCodeError
	if !errors.As(err, &ce) {
		return false
	}
	switch ce.Kind {
	case ClaudeCodeErrCrashed:
		return true
	case ClaudeCodeErrOther:
		return ce.Status >= 500 || strings.Contains(strings.ToLower(ce.Message), "overloaded")
	default:
		return false
	}
}

func firstClaudeCodeLine(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	const max = 300
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
