package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeClaudeShim installs a fake `claude` binary (a shell script) in a temp
// dir and points KAPI_CLAUDE_CODE_BIN at it. No test ever runs the real CLI.
func writeClaudeShim(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("claude shim scripts are POSIX shell")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755))
	t.Setenv("KAPI_CLAUDE_CODE_BIN", path)
	return path
}

// shimEnvelope returns a script that prints the given JSON envelope.
func shimEnvelope(envelope string) string {
	return fmt.Sprintf("cat >/dev/null\ncat <<'EOF'\n%s\nEOF\n", envelope)
}

const happyEnvelope = `{"type":"result","subtype":"success","is_error":false,"api_error_status":null,` +
	`"num_turns":1,"result":"Bonjour","stop_reason":"end_turn","session_id":"s","total_cost_usd":0.01,` +
	`"usage":{"input_tokens":9,"cache_creation_input_tokens":6100,"cache_read_input_tokens":42,"output_tokens":37},` +
	`"modelUsage":{"claude-haiku-4-5-20251001":{"inputTokens":9}}}`

func TestClaudeCodeChatHappyPath(t *testing.T) {
	writeClaudeShim(t, shimEnvelope(happyEnvelope))
	p := NewClaudeCodeProvider(Config{Model: "haiku"})

	resp, err := p.Chat(context.Background(), []Message{TextMessage("user", "Say bonjour")})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", resp.Content)
	assert.Equal(t, "claude-haiku-4-5-20251001", resp.Model)
	assert.Equal(t, 9, resp.Usage.InputTokens)
	assert.Equal(t, 37, resp.Usage.OutputTokens)
	assert.Equal(t, 6100, resp.Usage.CacheCreationTokens)
	assert.Equal(t, 42, resp.Usage.CacheReadTokens)
}

func TestClaudeCodeArgsAndStdin(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	stdinFile := filepath.Join(dir, "stdin")
	promptFile := filepath.Join(dir, "systemprompt")
	// Capture argv and stdin, and dereference the system-prompt file so the
	// test can assert on what the CLI would actually have read.
	capture := fmt.Sprintf(`printf '%%s\n' "$@" > %q
prev=""
for a in "$@"; do
  if [ "$prev" = "--append-system-prompt-file" ]; then cp "$a" %q; fi
  prev="$a"
done
cat > %q
`, argsFile, promptFile, stdinFile)
	writeClaudeShim(t, capture+shimEnvelope(happyEnvelope)[len("cat >/dev/null\n"):])

	p := NewClaudeCodeProvider(Config{Model: "sonnet"})
	_, err := p.Chat(context.Background(), []Message{
		TextMessage("system", "You translate UI strings."),
		TextMessage("user", "Say bonjour"),
	})
	require.NoError(t, err)

	args, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	got := string(args)
	assert.Contains(t, got, "-p\n")
	assert.Contains(t, got, "--output-format\njson\n")
	assert.Contains(t, got, "--model\nsonnet\n")
	assert.Contains(t, got, "--tools\n\n") // tool use disabled
	assert.Contains(t, got, "--no-session-persistence\n")
	assert.Contains(t, got, "--setting-sources\n\n")
	assert.Contains(t, got, "--append-system-prompt-file\n")

	// The system prompt is document-derived and must not be readable in a
	// process listing; only the path to it rides on argv.
	assert.NotContains(t, got, "You translate UI strings.",
		"the system prompt must not appear on the command line")

	system, err := os.ReadFile(promptFile)
	require.NoError(t, err, "the CLI must be handed a readable system-prompt file")
	assert.Equal(t, "You translate UI strings.", string(system))

	stdin, err := os.ReadFile(stdinFile)
	require.NoError(t, err)
	assert.Equal(t, "Say bonjour", string(stdin))
}

// TestClaudeCodeSystemPromptFileIsPrivateAndRemoved: the prompt is moved off
// argv to keep it away from other local accounts, so the file it moves to must
// be unreadable to them — and must not outlive the call.
func TestClaudeCodeSystemPromptFileIsPrivateAndRemoved(t *testing.T) {
	dir := t.TempDir()
	pathFile := filepath.Join(dir, "path")
	modeFile := filepath.Join(dir, "mode")
	capture := fmt.Sprintf(`prev=""
for a in "$@"; do
  if [ "$prev" = "--append-system-prompt-file" ]; then
    printf '%%s' "$a" > %q
    ls -l "$a" | cut -c1-10 > %q
  fi
  prev="$a"
done
cat >/dev/null
`, pathFile, modeFile)
	writeClaudeShim(t, capture+shimEnvelope(happyEnvelope)[len("cat >/dev/null\n"):])

	p := NewClaudeCodeProvider(Config{Model: "sonnet"})
	_, err := p.Chat(context.Background(), []Message{
		TextMessage("system", "secret instructions"),
		TextMessage("user", "go"),
	})
	require.NoError(t, err)

	mode, err := os.ReadFile(modeFile)
	require.NoError(t, err)
	assert.Equal(t, "-rw-------", strings.TrimSpace(string(mode)),
		"only the invoking user may read the system prompt")

	promptPath, err := os.ReadFile(pathFile)
	require.NoError(t, err)
	_, statErr := os.Stat(string(promptPath))
	assert.True(t, os.IsNotExist(statErr), "the system-prompt file is removed when the call returns")
}

// TestClaudeCodeNoSystemPromptWritesNoFile: the ordinary no-system-prompt call
// gains neither a flag nor a temp file.
func TestClaudeCodeNoSystemPromptWritesNoFile(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	writeClaudeShim(t, fmt.Sprintf("printf '%%s\\n' \"$@\" > %q\n", argsFile)+
		shimEnvelope(happyEnvelope))

	p := NewClaudeCodeProvider(Config{Model: "sonnet"})
	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "go")})
	require.NoError(t, err)

	args, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.NotContains(t, string(args), "--append-system-prompt")
}

func TestClaudeCodeStructuredOutput(t *testing.T) {
	envelope := `{"type":"result","subtype":"success","is_error":false,"num_turns":2,"result":"Done.",` +
		`"usage":{"input_tokens":17,"output_tokens":258},` +
		`"structured_output":{"translations":[{"index":1,"text":"Bonjour"}]}}`
	writeClaudeShim(t, shimEnvelope(envelope))
	p := NewClaudeCodeProvider(Config{})

	resp, err := p.ChatStructured(context.Background(),
		[]Message{TextMessage("user", "translate")},
		JSONSchema{Name: "batch_translations", Schema: map[string]any{"type": "object"}})
	require.NoError(t, err)

	var parsed struct {
		Translations []struct {
			Index int    `json:"index"`
			Text  string `json:"text"`
		} `json:"translations"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &parsed))
	require.Len(t, parsed.Translations, 1)
	assert.Equal(t, "Bonjour", parsed.Translations[0].Text)
}

func TestClaudeCodeLimitErrorMapping(t *testing.T) {
	resetUnix := time.Now().Add(2 * time.Hour).Unix()
	envelope := fmt.Sprintf(`{"type":"result","subtype":"success","is_error":true,"api_error_status":429,`+
		`"result":"Claude AI usage limit reached|%d","usage":{"input_tokens":0,"output_tokens":0}}`, resetUnix)
	writeClaudeShim(t, shimEnvelope(envelope))
	p := NewClaudeCodeProvider(Config{})

	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	var ce *ClaudeCodeError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, ClaudeCodeErrLimit, ce.Kind)
	assert.NotEmpty(t, ce.ResetsAt, "unix reset timestamp should parse")
	assert.Contains(t, err.Error(), "Claude subscription limit reached")
	assert.Contains(t, err.Error(), "resuming ~")
	assert.Contains(t, err.Error(), "kapi models setup")
}

func TestClaudeCodeLimitErrorPhraseVariants(t *testing.T) {
	for _, msg := range []string{
		"You've hit your limit · resets 3am",
		"5-hour usage limit reached, your limit will reset at 10:30pm (Europe/Oslo)",
		"You are out of extra usage",
	} {
		ce := classifyClaudeCodeMessage(0, msg)
		require.NotNil(t, ce, "message %q should classify", msg)
		assert.Equal(t, ClaudeCodeErrLimit, ce.Kind, msg)
	}
	assert.Equal(t, "3am", classifyClaudeCodeMessage(0, "You've hit your limit · resets 3am").ResetsAt)
	assert.Equal(t, "10:30pm (Europe/Oslo)",
		parseClaudeCodeReset("your limit will reset at 10:30pm (Europe/Oslo)"))
}

func TestClaudeCodeAuthErrorMapping(t *testing.T) {
	envelope := `{"type":"result","subtype":"success","is_error":true,"api_error_status":401,` +
		`"result":"OAuth token has expired. Please run /login.","usage":{"input_tokens":0,"output_tokens":0}}`
	writeClaudeShim(t, shimEnvelope(envelope))
	p := NewClaudeCodeProvider(Config{})

	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	var ce *ClaudeCodeError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, ClaudeCodeErrAuth, ce.Kind)
	assert.Equal(t, "Claude Code isn't signed in. Run `claude` once to log in.", err.Error())
}

func TestClaudeCodeAuthErrorFromStderr(t *testing.T) {
	// Pre-flight failures exit non-zero with plain text, no JSON envelope.
	writeClaudeShim(t, "cat >/dev/null\necho 'Not logged in. Please run claude login.' >&2\nexit 1\n")
	p := NewClaudeCodeProvider(Config{})

	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	var ce *ClaudeCodeError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, ClaudeCodeErrAuth, ce.Kind)
}

func TestClaudeCodeTimeout(t *testing.T) {
	writeClaudeShim(t, "cat >/dev/null\nsleep 30\n")
	t.Setenv("KAPI_CLAUDE_CODE_TIMEOUT", "100ms")
	p := NewClaudeCodeProvider(Config{})

	start := time.Now()
	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 10*time.Second)
	var ce *ClaudeCodeError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, ClaudeCodeErrTimeout, ce.Kind)
	assert.Contains(t, err.Error(), "KAPI_CLAUDE_CODE_TIMEOUT")
}

func TestClaudeCodeRetryOnTransientCrash(t *testing.T) {
	// First call crashes without an envelope; second succeeds. The counter
	// file makes the shim stateful.
	dir := t.TempDir()
	marker := filepath.Join(dir, "attempted")
	script := fmt.Sprintf(`cat >/dev/null
if [ ! -f %q ]; then
  touch %q
  echo boom >&2
  exit 1
fi
cat <<'EOF'
%s
EOF
`, marker, marker, happyEnvelope)
	writeClaudeShim(t, script)
	p := NewClaudeCodeProvider(Config{})

	resp, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.NoError(t, err, "one transient crash should be retried")
	assert.Equal(t, "Bonjour", resp.Content)
}

func TestClaudeCodeNoRetryOnLimit(t *testing.T) {
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count")
	script := fmt.Sprintf(`cat >/dev/null
echo x >> %q
cat <<'EOF'
{"type":"result","is_error":true,"api_error_status":429,"result":"usage limit reached","usage":{"input_tokens":0,"output_tokens":0}}
EOF
`, countFile)
	writeClaudeShim(t, script)
	p := NewClaudeCodeProvider(Config{})

	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	data, readErr := os.ReadFile(countFile)
	require.NoError(t, readErr)
	assert.Equal(t, "x\n", string(data), "limit errors must not retry")
}

func TestClaudeCodeNotInstalled(t *testing.T) {
	t.Setenv("KAPI_CLAUDE_CODE_BIN", filepath.Join(t.TempDir(), "definitely-missing"))
	p := NewClaudeCodeProvider(Config{})

	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	var ce *ClaudeCodeError
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, ClaudeCodeErrNotInstalled, ce.Kind)

	assert.False(t, ClaudeCodeDetected())
}

func TestClaudeCodeDetected(t *testing.T) {
	writeClaudeShim(t, shimEnvelope(happyEnvelope))
	assert.True(t, ClaudeCodeDetected())
	path, ok := ClaudeCodeBinaryPath()
	assert.True(t, ok)
	assert.NotEmpty(t, path)
}

func TestClaudeCodeTranslateUsesChat(t *testing.T) {
	writeClaudeShim(t, shimEnvelope(happyEnvelope))
	p := NewClaudeCodeProvider(Config{})

	resp, err := p.Translate(context.Background(), TranslateRequest{
		Source: "Hello", SourceLanguage: "en", TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", resp.Translation)
	assert.InDelta(t, 0.85, resp.Confidence, 0.001)
}

func TestClaudeCodeRegistration(t *testing.T) {
	info, ok := ProviderInfoFor(ClaudeCode)
	require.True(t, ok)
	assert.True(t, info.Keyless)
	assert.True(t, info.Subscription)
	assert.False(t, info.Local, "claude-code egresses content — never local")
	assert.False(t, info.NeedsKey())
	assert.Equal(t, DefaultClaudeCodeModel, info.DefaultModel)

	p, err := NewProvider(ClaudeCode, Config{})
	require.NoError(t, err)
	assert.Equal(t, ClaudeCode, p.Name())

	// Model inference must keep claude-* ids on the API provider.
	id, ok := ProviderForModel("claude-sonnet-4-20250514")
	require.True(t, ok)
	assert.Equal(t, Anthropic, id)
}

func TestClaudeCodeErrorRendering(t *testing.T) {
	assert.Contains(t, (&ClaudeCodeError{Kind: ClaudeCodeErrNotInstalled}).Error(), "kapi models setup")
	assert.Contains(t, (&ClaudeCodeError{Kind: ClaudeCodeErrCrashed, Message: "boom"}).Error(), "boom")
	generic := &ClaudeCodeError{Kind: ClaudeCodeErrOther, Message: "bad", Status: 500}
	assert.Contains(t, generic.Error(), "500")
	assert.True(t, claudeCodeTransient(generic))
	assert.False(t, claudeCodeTransient(errors.New("plain")))
}
