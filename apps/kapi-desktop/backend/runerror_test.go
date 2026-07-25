package backend

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/credentials"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// wrapLikeConverge reproduces the wrapping the real stack applies to a provider
// failure during `kapi up`: provider → translate tool → executor → file runner →
// multi-file runner → convergence engine. Classification must survive all of it.
func wrapLikeConverge(locale, file string, cause error) error {
	err := fmt.Errorf("translate: %w", cause)
	err = fmt.Errorf("tool translate: %w", err)
	err = fmt.Errorf("execute flow: %w", err)
	err = fmt.Errorf("%s: %w", file, err)
	err = fmt.Errorf("flow execution error: %w", err)
	return fmt.Errorf("converge %s: %w", locale, err)
}

func TestClassifyRunError_ModelNotInstalled(t *testing.T) {
	cause := &aiprovider.OllamaError{
		Kind:   aiprovider.OllamaErrModelNotInstalled,
		Model:  "gemma4:e2b",
		Detail: `{"error":"model 'gemma4:e2b' not found"}`,
	}
	err := wrapLikeConverge("nb", "/Users/dev/notes/release-notes.md", cause)

	re := classifyRunError(err)
	require.NotNil(t, re)

	assert.Equal(t, RunErrModelNotInstalled, re.Kind)
	// A short human headline — not the wrapped chain.
	assert.Equal(t, "Model gemma4:e2b isn't installed", re.Headline)
	assert.NotContains(t, re.Headline, "flow execution error")
	assert.NotContains(t, re.Headline, "execute flow")

	// The remedy is structure, not prose to be parsed out of the message.
	require.NotEmpty(t, re.Actions)
	assert.Equal(t, ActionCommand, re.Actions[0].Kind)
	assert.Equal(t, "ollama pull gemma4:e2b", re.Actions[0].Command)

	// Affected scope as metadata.
	assert.Equal(t, "nb", re.Locale)
	assert.Equal(t, "/Users/dev/notes/release-notes.md", re.File)
	assert.Equal(t, "ollama", re.Provider)
	assert.Equal(t, "gemma4:e2b", re.Model)

	// The raw chain is preserved verbatim for the Details disclosure.
	assert.Equal(t, err.Error(), re.Raw)
	assert.Contains(t, re.Raw, "flow execution error")
}

func TestClassifyRunError_OllamaUnreachable(t *testing.T) {
	cause := &aiprovider.OllamaError{
		Kind:    aiprovider.OllamaErrUnreachable,
		BaseURL: "http://localhost:11434",
		Err:     fmt.Errorf(`Post "http://localhost:11434/api/chat": %w`, context.DeadlineExceeded),
	}
	err := wrapLikeConverge("nb", "notes.md", cause)

	re := classifyRunError(err)
	require.NotNil(t, re)
	assert.Equal(t, RunErrOllamaUnreachable, re.Kind)
	assert.Equal(t, "Ollama isn't running", re.Headline)
	assert.Contains(t, re.Remediation, "http://localhost:11434")

	require.Len(t, re.Actions, 2)
	assert.Equal(t, ActionCommand, re.Actions[0].Kind)
	assert.Equal(t, "ollama serve", re.Actions[0].Command)
	assert.Equal(t, ActionOpenURL, re.Actions[1].Kind)
	assert.Equal(t, "https://ollama.com", re.Actions[1].URL)

	assert.Equal(t, "nb", re.Locale)
	assert.Equal(t, "notes.md", re.File)
	// The transport cause stays reachable through the chain.
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClassifyRunError_Canceled(t *testing.T) {
	re := classifyRunError(fmt.Errorf("converge nb: %w", context.Canceled))
	require.NotNil(t, re)
	assert.Equal(t, RunErrCanceled, re.Kind)
	assert.Equal(t, "Run canceled", re.Headline)
	assert.Empty(t, re.Actions)
}

func TestClassifyRunError_AmbiguousCredential(t *testing.T) {
	cause := &credentials.AmbiguousCredentialError{
		Provider:   "anthropic",
		Candidates: []string{"work", "personal"},
	}
	re := classifyRunError(fmt.Errorf("tool translate: %w", cause))
	require.NotNil(t, re)
	assert.Equal(t, RunErrAmbiguousCredential, re.Kind)
	assert.Equal(t, "More than one AI credential matches", re.Headline)
	assert.Contains(t, re.Remediation, "work")
	assert.Contains(t, re.Remediation, "personal")
	require.Len(t, re.Actions, 1)
	assert.Equal(t, ActionOpenSettings, re.Actions[0].Kind)
	assert.Equal(t, "settings", re.Actions[0].Target)
}

func TestClassifyRunError_MissingCredential(t *testing.T) {
	tb := &host.FlowToolBuildError{
		Tool:   "translate",
		Locale: "de",
		Err:    errors.New("no saved credentials for provider anthropic"),
	}
	re := classifyRunError(fmt.Errorf("flow %q: %w", "translate", tb))
	require.NotNil(t, re)
	assert.Equal(t, RunErrMissingCredential, re.Kind)
	assert.Contains(t, re.Headline, "translate")
	assert.Equal(t, "de", re.Locale)
	require.Len(t, re.Actions, 1)
	assert.Equal(t, ActionOpenSettings, re.Actions[0].Kind)
}

func TestClassifyRunError_UnknownKeepsMessageAndNeverSwallows(t *testing.T) {
	re := classifyRunError(errors.New("something entirely novel broke\nsecond line"))
	require.NotNil(t, re)
	assert.Equal(t, RunErrUnknown, re.Kind)
	// The headline is one line; the rest stays in Raw.
	assert.Equal(t, "something entirely novel broke", re.Headline)
	assert.Contains(t, re.Raw, "second line")
	assert.Empty(t, re.Actions)
}

func TestClassifyRunError_Nil(t *testing.T) {
	assert.Nil(t, classifyRunError(nil))
}

func TestRunErrorScope_DesktopFlowRunnerShape(t *testing.T) {
	// host/flowrun.go renders per-file failures as "<file> [<locale>]: <cause>".
	locale, file := runErrorScope(errors.New("messages.json [fr]: execute flow: boom"))
	assert.Equal(t, "fr", locale)
	assert.Equal(t, "messages.json", file)
}

func TestRunErrorScope_NoScope(t *testing.T) {
	locale, file := runErrorScope(errors.New("plain failure"))
	assert.Empty(t, locale)
	assert.Empty(t, file)
}

func TestClassifyRunError_SingleFileRunStillClassifies(t *testing.T) {
	// A single-file run skips the multi-file runner, so the chain carries no
	// `flow execution error: <file>:` prefix — there is no file to report. The
	// missing metadata must cost only that field: the classification and the
	// remedy still have to come through, since they are read off the typed error
	// rather than parsed out of the text.
	cause := &aiprovider.OllamaError{
		Kind:    aiprovider.OllamaErrUnreachable,
		BaseURL: "http://localhost:11434",
		Err:     errors.New("connection refused"),
	}
	err := fmt.Errorf("converge nb: %w",
		fmt.Errorf("execute flow: %w",
			fmt.Errorf("tool translate: %w", fmt.Errorf("translate: %w", cause))))

	re := classifyRunError(err)
	require.NotNil(t, re)
	assert.Equal(t, RunErrOllamaUnreachable, re.Kind)
	assert.Equal(t, "Ollama isn't running", re.Headline)
	assert.Equal(t, "nb", re.Locale)
	assert.Empty(t, re.File, "no file in the chain — the field is absent, not guessed")
	require.NotEmpty(t, re.Actions)
	assert.Equal(t, "ollama serve", re.Actions[0].Command)
}

func TestGetLastRunError_RecordsAndClears(t *testing.T) {
	a := &App{runState: newRunner()}
	assert.Nil(t, a.GetLastRunError(), "no run attempted yet")

	cause := &aiprovider.OllamaError{
		Kind:  aiprovider.OllamaErrModelNotInstalled,
		Model: "gemma4:e2b",
	}
	a.recordRunFailure(classifyRunError(wrapLikeConverge("nb", "a.md", cause)))

	got := a.GetLastRunError()
	require.NotNil(t, got)
	assert.Equal(t, RunErrModelNotInstalled, got.Kind)

	// A subsequent success clears it — a stale failure must not keep marking a
	// converged project as broken.
	a.recordRunFailure(nil)
	assert.Nil(t, a.GetLastRunError())
}

func TestRecordRunFailure_NilRunnerIsSafe(t *testing.T) {
	a := &App{}
	a.recordRunFailure(&RunError{Kind: RunErrUnknown})
	assert.Nil(t, a.GetLastRunError())
}
