package mtprovider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	mttools "github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/tool"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processPart is a helper that sends a single Part through a tool and returns the result.
func processPart(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	return result
}

func newTool(p mtprovider.MTProvider, source, target model.LocaleID) *mttools.MTTranslateTool {
	return mttools.NewMTTranslateTool(p, mttools.MTTranslateConfig{
		SourceLocale: source,
		TargetLocale: target,
	})
}

func TestGoogleProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/language/translate/v2", r.URL.Path)
		// The API key must travel in the X-Goog-Api-Key header, never in the
		// URL query string (avoids leaking the secret into wrapped errors/logs).
		assert.Equal(t, "test-key", r.Header.Get("X-Goog-Api-Key"))
		assert.Empty(t, r.URL.Query().Get("key"))
		assert.Equal(t, http.MethodPost, r.Method)

		var reqBody struct {
			Q      string `json:"q"`
			Source string `json:"source"`
			Target string `json:"target"`
			Format string `json:"format"`
		}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if assert.NoError(t, err) {
			assert.Equal(t, "Hello world", reqBody.Q)
			assert.Equal(t, "en", reqBody.Source)
			assert.Equal(t, "fr", reqBody.Target)
		}

		resp := map[string]any{
			"data": map[string]any{
				"translations": []map[string]any{
					{"translatedText": "Bonjour le monde"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := mtprovider.NewGoogleProvider(mtprovider.GoogleConfig{
		APIKey:    "test-key",
		ProjectID: "test-project",
		BaseURL:   server.URL,
	})
	tl := newTool(p, model.LocaleEnglish, model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "Hello world", resultBlock.SourceText())
}

func TestGoogleProviderSkipsNonTranslatable(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Errorf("API should not be called for non-translatable blocks (got request to %s)", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := mtprovider.NewGoogleProvider(mtprovider.GoogleConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	tl := newTool(p, "", model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	require.Zero(t, hits.Load(), "provider must not call the API for non-translatable blocks")
}

func TestDeepLProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/translate", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "DeepL-Auth-Key")
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		if assert.NoError(t, err) {
			assert.Equal(t, "Hello world", r.FormValue("text"))
			assert.Equal(t, "FR", r.FormValue("target_lang"))
			assert.Equal(t, "EN", r.FormValue("source_lang"))
			assert.Equal(t, "more", r.FormValue("formality"))
		}

		resp := map[string]any{
			"translations": []map[string]any{
				{
					"detected_source_language": "EN",
					"text":                     "Bonjour le monde",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := mtprovider.NewDeepLProvider(mtprovider.DeepLConfig{
		APIKey:    "test-deepl-key",
		Formality: mtprovider.FormalityMore,
		BaseURL:   server.URL,
	})
	tl := newTool(p, model.LocaleEnglish, model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestDeepLProviderSkipsNonTranslatable(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Errorf("API should not be called for non-translatable blocks (got request to %s)", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := mtprovider.NewDeepLProvider(mtprovider.DeepLConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	tl := newTool(p, "", model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	require.Zero(t, hits.Load(), "provider must not call the API for non-translatable blocks")
}

func TestMicrosoftProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/translate", r.URL.Path)
		assert.Equal(t, "3.0", r.URL.Query().Get("api-version"))
		assert.Equal(t, "fr", r.URL.Query().Get("to"))
		assert.Equal(t, "en", r.URL.Query().Get("from"))
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "test-sub-key", r.Header.Get("Ocp-Apim-Subscription-Key"))
		assert.Equal(t, "westeurope", r.Header.Get("Ocp-Apim-Subscription-Region"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody []struct {
			Text string `json:"Text"`
		}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if assert.NoError(t, err) {
			assert.Len(t, reqBody, 1)
			if len(reqBody) > 0 {
				assert.Equal(t, "Hello world", reqBody[0].Text)
			}
		}

		resp := []map[string]any{
			{
				"translations": []map[string]any{
					{"text": "Bonjour le monde", "to": "fr"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := mtprovider.NewMicrosoftProvider(mtprovider.MicrosoftConfig{
		SubscriptionKey: "test-sub-key",
		Region:          "westeurope",
		BaseURL:         server.URL,
	})
	tl := newTool(p, model.LocaleEnglish, model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestMicrosoftProviderSkipsNonTranslatable(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Errorf("API should not be called for non-translatable blocks (got request to %s)", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := mtprovider.NewMicrosoftProvider(mtprovider.MicrosoftConfig{
		SubscriptionKey: "test-key",
		BaseURL:         server.URL,
	})
	tl := newTool(p, "", model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	require.Zero(t, hits.Load(), "provider must not call the API for non-translatable blocks")
}

func TestMyMemoryProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/get", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Hello world", r.URL.Query().Get("q"))
		assert.Equal(t, "en|fr", r.URL.Query().Get("langpair"))
		assert.Equal(t, "test@example.com", r.URL.Query().Get("de"))

		resp := map[string]any{
			"responseData": map[string]any{
				"translatedText": "Bonjour le monde",
				"match":          0.95,
			},
			"responseStatus": 200,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := mtprovider.NewMyMemoryProvider(mtprovider.MyMemoryConfig{
		Email:   "test@example.com",
		BaseURL: server.URL,
	})
	tl := newTool(p, model.LocaleEnglish, model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestMyMemoryProviderSkipsNonTranslatable(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Errorf("API should not be called for non-translatable blocks (got request to %s)", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := mtprovider.NewMyMemoryProvider(mtprovider.MyMemoryConfig{
		BaseURL: server.URL,
	})
	tl := newTool(p, model.LocaleEnglish, model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	require.Zero(t, hits.Load(), "provider must not call the API for non-translatable blocks")
}

func TestModernMTProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/translate", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "test-mmt-key", r.Header.Get("MMT-ApiKey"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var reqBody struct {
			Source string  `json:"source"`
			Target string  `json:"target"`
			Q      string  `json:"q"`
			Hints  []int64 `json:"hints"`
		}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if assert.NoError(t, err) {
			assert.Equal(t, "Hello world", reqBody.Q)
			assert.Equal(t, "en", reqBody.Source)
			assert.Equal(t, "fr", reqBody.Target)
		}

		resp := map[string]any{
			"status": 200,
			"data": map[string]any{
				"translation": "Bonjour le monde",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := mtprovider.NewModernMTProvider(mtprovider.ModernMTConfig{
		APIKey:  "test-mmt-key",
		BaseURL: server.URL,
	})
	tl := newTool(p, model.LocaleEnglish, model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestModernMTProviderSkipsNonTranslatable(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Errorf("API should not be called for non-translatable blocks (got request to %s)", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := mtprovider.NewModernMTProvider(mtprovider.ModernMTConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	tl := newTool(p, "", model.LocaleFrench)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	require.Zero(t, hits.Load(), "provider must not call the API for non-translatable blocks")
}

func TestGoogleConfigValidation(t *testing.T) {
	cfg := &mtprovider.GoogleToolConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")

	cfg.APIKey = "key"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestDeepLConfigValidation(t *testing.T) {
	cfg := &mtprovider.DeepLToolConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")

	cfg.APIKey = "key"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestMicrosoftConfigValidation(t *testing.T) {
	cfg := &mtprovider.MicrosoftToolConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SubscriptionKey")

	cfg.SubscriptionKey = "key"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestMyMemoryConfigValidation(t *testing.T) {
	cfg := &mtprovider.MyMemoryToolConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SourceLocale")

	cfg.SourceLocale = model.LocaleEnglish
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

func TestModernMTConfigValidation(t *testing.T) {
	cfg := &mtprovider.ModernMTToolConfig{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")

	cfg.APIKey = "key"
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	require.NoError(t, err)
}

// Verify that all providers implement the MTProvider interface.
var _ mtprovider.MTProvider = (*mtprovider.DeepLProvider)(nil)
var _ mtprovider.MTProvider = (*mtprovider.GoogleProvider)(nil)
var _ mtprovider.MTProvider = (*mtprovider.MicrosoftProvider)(nil)
var _ mtprovider.MTProvider = (*mtprovider.ModernMTProvider)(nil)
var _ mtprovider.MTProvider = (*mtprovider.MyMemoryProvider)(nil)

// Verify the tool implements the Tool interface via BaseTool.
var _ tool.Tool = (*mttools.MTTranslateTool)(nil)
