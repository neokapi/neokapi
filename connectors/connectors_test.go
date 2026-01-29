package connectors_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asgeirf/gokapi/connectors/deepl"
	"github.com/asgeirf/gokapi/connectors/google"
	"github.com/asgeirf/gokapi/connectors/microsoft"
	"github.com/asgeirf/gokapi/connectors/modernmt"
	"github.com/asgeirf/gokapi/connectors/mymemory"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processPart is a helper that sends a single Part through a tool and returns the result.
func processPart(t *testing.T, tool interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tool.Process(context.Background(), in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	return result
}

func TestGoogleTranslateConnector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/language/translate/v2", r.URL.Path)
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))
		assert.Equal(t, http.MethodPost, r.Method)

		var reqBody struct {
			Q      string `json:"q"`
			Source string `json:"source"`
			Target string `json:"target"`
			Format string `json:"format"`
		}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)
		assert.Equal(t, "Hello world", reqBody.Q)
		assert.Equal(t, "en", reqBody.Source)
		assert.Equal(t, "fr", reqBody.Target)

		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"translations": []map[string]interface{}{
					{"translatedText": "Bonjour le monde"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &google.GoogleConfig{
		APIKey:       "test-key",
		ProjectID:    "test-project",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := google.NewGoogleTranslateConnector(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "Hello world", resultBlock.SourceText())
}

func TestGoogleTranslateConnectorSkipsNonTranslatable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for non-translatable blocks")
	}))
	defer server.Close()

	cfg := &google.GoogleConfig{
		APIKey:       "test-key",
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := google.NewGoogleTranslateConnector(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestDeepLConnector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/translate", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "DeepL-Auth-Key")
		assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

		err := r.ParseForm()
		require.NoError(t, err)
		assert.Equal(t, "Hello world", r.FormValue("text"))
		assert.Equal(t, "FR", r.FormValue("target_lang"))
		assert.Equal(t, "EN", r.FormValue("source_lang"))
		assert.Equal(t, "more", r.FormValue("formality"))

		resp := map[string]interface{}{
			"translations": []map[string]interface{}{
				{
					"detected_source_language": "EN",
					"text":                     "Bonjour le monde",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &deepl.DeepLConfig{
		APIKey:       "test-deepl-key",
		Formality:    deepl.FormalityMore,
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := deepl.NewDeepLConnector(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestDeepLConnectorSkipsNonTranslatable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for non-translatable blocks")
	}))
	defer server.Close()

	cfg := &deepl.DeepLConfig{
		APIKey:       "test-key",
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := deepl.NewDeepLConnector(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestMicrosoftTranslatorConnector(t *testing.T) {
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
		require.NoError(t, err)
		require.Len(t, reqBody, 1)
		assert.Equal(t, "Hello world", reqBody[0].Text)

		resp := []map[string]interface{}{
			{
				"translations": []map[string]interface{}{
					{"text": "Bonjour le monde", "to": "fr"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &microsoft.MSConfig{
		SubscriptionKey: "test-sub-key",
		Region:          "westeurope",
		SourceLocale:    model.LocaleEnglish,
		TargetLocale:    model.LocaleFrench,
		BaseURL:         server.URL,
	}
	connector := microsoft.NewMicrosoftTranslatorConnector(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestMicrosoftTranslatorConnectorSkipsNonTranslatable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for non-translatable blocks")
	}))
	defer server.Close()

	cfg := &microsoft.MSConfig{
		SubscriptionKey: "test-key",
		TargetLocale:    model.LocaleFrench,
		BaseURL:         server.URL,
	}
	connector := microsoft.NewMicrosoftTranslatorConnector(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestMyMemoryConnector(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/get", r.URL.Path)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Hello world", r.URL.Query().Get("q"))
		assert.Equal(t, "en|fr", r.URL.Query().Get("langpair"))
		assert.Equal(t, "test@example.com", r.URL.Query().Get("de"))

		resp := map[string]interface{}{
			"responseData": map[string]interface{}{
				"translatedText": "Bonjour le monde",
				"match":          0.95,
			},
			"responseStatus": 200,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &mymemory.MyMemoryConfig{
		Email:        "test@example.com",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := mymemory.NewMyMemoryConnector(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestMyMemoryConnectorSkipsNonTranslatable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for non-translatable blocks")
	}))
	defer server.Close()

	cfg := &mymemory.MyMemoryConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := mymemory.NewMyMemoryConnector(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestGoogleConfigValidation(t *testing.T) {
	cfg := &google.GoogleConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")

	cfg.APIKey = "key"
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestDeepLConfigValidation(t *testing.T) {
	cfg := &deepl.DeepLConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")

	cfg.APIKey = "key"
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestMSConfigValidation(t *testing.T) {
	cfg := &microsoft.MSConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SubscriptionKey")

	cfg.SubscriptionKey = "key"
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestMyMemoryConfigValidation(t *testing.T) {
	cfg := &mymemory.MyMemoryConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SourceLocale")

	cfg.SourceLocale = model.LocaleEnglish
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}

func TestModernMTConnector(t *testing.T) {
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
		require.NoError(t, err)
		assert.Equal(t, "Hello world", reqBody.Q)
		assert.Equal(t, "en", reqBody.Source)
		assert.Equal(t, "fr", reqBody.Target)

		resp := map[string]interface{}{
			"status": 200,
			"data": map[string]interface{}{
				"translation": "Bonjour le monde",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &modernmt.ModernMTConfig{
		APIKey:       "test-mmt-key",
		SourceLocale: model.LocaleEnglish,
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := modernmt.NewModernMTConnector(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestModernMTConnectorSkipsNonTranslatable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("API should not be called for non-translatable blocks")
	}))
	defer server.Close()

	cfg := &modernmt.ModernMTConfig{
		APIKey:       "test-key",
		TargetLocale: model.LocaleFrench,
		BaseURL:      server.URL,
	}
	connector := modernmt.NewModernMTConnector(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, connector, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestModernMTConfigValidation(t *testing.T) {
	cfg := &modernmt.ModernMTConfig{}
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "APIKey")

	cfg.APIKey = "key"
	err = cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = model.LocaleFrench
	err = cfg.Validate()
	assert.NoError(t, err)
}
