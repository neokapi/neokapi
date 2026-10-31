package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/credentials/providerenv"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
)

// `kapi voice check --ai` and `kapi terms expand` ask a model outside a run of
// a registry tool. Under a declared redaction policy neither request holds a
// value the policy withholds; with no policy each sends its text as it is.

// isolateEgress keeps the commands off the developer's configuration and
// every provider key, so the demo provider a command names is the only one
// it reaches.
func isolateEgress(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", "")
	t.Setenv("KAPI_NO_PROJECT", "1")
	for _, vars := range providerenv.Vars {
		for _, v := range vars {
			t.Setenv(v, "")
		}
	}
	t.Setenv("KAPI_AI_PROVIDER", "")
	t.Setenv("KAPI_AI_MODEL", "")
	aiprovider.SetDemoNoticeWriter(io.Discard)
}

// egressSent records every request a provider is sent while the test runs,
// as JSON without the provider's answer.
func egressSent(t *testing.T) func() string {
	t.Helper()
	var (
		mu   sync.Mutex
		sent strings.Builder
	)
	remove := aiprovider.AddRecorder(func(_ context.Context, ex aiprovider.Exchange) {
		ex.Response = ""
		raw, _ := json.Marshal(ex)
		mu.Lock()
		defer mu.Unlock()
		sent.Write(raw)
		sent.WriteString("\n")
	})
	t.Cleanup(remove)
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		return sent.String()
	}
}

// egressRecipe writes a project whose default redaction policy withholds
// Ada Lovelace, and returns its recipe.
func egressRecipe(t *testing.T) string {
	t.Helper()
	root := writeProjectRecipe(t, `version: v1
name: egress
defaults:
  source_language: en
  target_languages: [nb]
  redaction:
    enabled: true
    rules: redact.yaml
    detectors: [rules]
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "redact.yaml"),
		[]byte("version: v1\nrules:\n  - term: Ada Lovelace\n    category: person\n"), 0o600))
	return filepath.Join(root, "kapi.yaml")
}

func TestVoiceCheckAI_RedactsUnderTheProjectsPolicy(t *testing.T) {
	isolateEgress(t)
	profile := filepath.Join(t.TempDir(), "voice.yaml")
	require.NoError(t, os.WriteFile(profile, []byte(`name: Egress
tone:
  formality: formal
  guidelines: Greet Ada Lovelace by her full name.
style:
  active_voice: true
`), 0o600))
	run := func(t *testing.T, extra ...string) string {
		t.Helper()
		sent := egressSent(t)
		cmd := newVoiceCheckCmd(&App{SourceLang: "en"})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"--profile-file", profile, "--ai", "--provider", "demo",
			"--input-text", "Write to Ada Lovelace today.", "--json"}, extra...))
		if err := cmd.Execute(); err != nil && !errors.Is(err, ErrQualityGate) {
			require.NoError(t, err)
		}
		return sent()
	}

	t.Run("under a policy", func(t *testing.T) {
		sent := run(t, "-p", egressRecipe(t))
		require.Contains(t, sent, "Write to [REDACTED:Person] today.", "the model read the text")
		assert.NotContains(t, sent, "Lovelace", "neither the text nor the voice profile carries the name")
	})
	t.Run("with no policy", func(t *testing.T) {
		assert.Contains(t, run(t), "Write to Ada Lovelace today.", "the text goes as it is")
	})
}

func TestTermsExpand_RedactsUnderTheProjectsDefaultPolicy(t *testing.T) {
	isolateEgress(t)
	run := func(t *testing.T, extra ...string) string {
		t.Helper()
		path := writeTermsBundle(t, []terms.Concept{
			{ID: "prize", Terms: []terms.Term{
				{Text: "Ada Lovelace Prize", Locale: "en"},
				{Text: "Ada Lovelace-prisen", Locale: "nb"},
			}},
			{ID: "alert", Terms: []terms.Term{{Text: "alert", Locale: "en"}, {Text: "varsel", Locale: "nb"}}},
		})
		sent := egressSent(t)
		_, err := runTermsCmd(t, &App{}, append([]string{"expand", path, "-s", "en", "--provider", "demo", "--dry-run"}, extra...)...)
		require.NoError(t, err)
		return sent()
	}

	t.Run("under a policy", func(t *testing.T) {
		sent := run(t, "-p", egressRecipe(t))
		require.Contains(t, sent, "varsel", "the model read the terms")
		assert.NotContains(t, sent, "Lovelace", "the withheld name goes out as its token")
		assert.Contains(t, sent, "[REDACTED:Person]-prisen")
	})
	t.Run("with no policy", func(t *testing.T) {
		assert.Contains(t, run(t), "Ada Lovelace-prisen", "the terms go as they are")
	})
}
