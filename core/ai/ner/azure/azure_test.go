package azure

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_RequiresEndpoint(t *testing.T) {
	_, err := New(ner.Config{APIKey: "key"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint")
}

func TestNew_RequiresAPIKey(t *testing.T) {
	_, err := New(ner.Config{Endpoint: "https://example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key")
}

func TestProvider_Name(t *testing.T) {
	p, err := New(ner.Config{Endpoint: "https://example.com", APIKey: "key"})
	require.NoError(t, err)
	assert.Equal(t, "azure", p.Name())
}

func TestProvider_SupportedLocales(t *testing.T) {
	p, err := New(ner.Config{Endpoint: "https://example.com", APIKey: "key"})
	require.NoError(t, err)
	assert.Nil(t, p.SupportedLocales())
}

func TestProvider_DetectEntities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("Ocp-Apim-Subscription-Key"))
		assert.Contains(t, r.URL.Path, "/language/:analyze-text")

		resp := analyzeResponse{
			Results: analyzeResults{
				Documents: []documentResult{
					{
						ID: "0",
						Entities: []azureEntity{
							{Text: "John Smith", Type: "Person", Offset: 0, Length: 10, ConfidenceScore: 0.95},
							{Text: "March 15, 2026", Type: "Date", Offset: 20, Length: 14, ConfidenceScore: 0.99},
							{Text: "$49.99", Type: "Currency", Offset: 40, Length: 6, ConfidenceScore: 0.98},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	p, err := New(ner.Config{Endpoint: server.URL, APIKey: "test-key"})
	require.NoError(t, err)

	resp, err := p.DetectEntities(t.Context(), ner.Request{
		Text:   "John Smith ordered on March 15, 2026 for $49.99",
		Locale: "en-US",
	})
	require.NoError(t, err)
	require.Len(t, resp.Entities, 3)

	assert.Equal(t, "John Smith", resp.Entities[0].Text)
	assert.Equal(t, model.EntityPerson, resp.Entities[0].Type)
	assert.Equal(t, 0.95, resp.Entities[0].Confidence)

	assert.Equal(t, "March 15, 2026", resp.Entities[1].Text)
	assert.Equal(t, model.EntityDate, resp.Entities[1].Type)

	assert.Equal(t, "$49.99", resp.Entities[2].Text)
	assert.Equal(t, model.EntityCurrency, resp.Entities[2].Type)
}

func TestProvider_DetectEntitiesBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req analyzeRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		results := make([]documentResult, len(req.AnalysisInput.Documents))
		for i, doc := range req.AnalysisInput.Documents {
			results[i] = documentResult{
				ID: doc.ID,
				Entities: []azureEntity{
					{Text: "entity-" + doc.ID, Type: "Person", Offset: 0, Length: 5, ConfidenceScore: 0.9},
				},
			}
		}

		resp := analyzeResponse{Results: analyzeResults{Documents: results}}
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	p, err := New(ner.Config{Endpoint: server.URL, APIKey: "key"})
	require.NoError(t, err)

	reqs := []ner.Request{
		{Text: "Hello World", Locale: "en"},
		{Text: "Bonjour le monde", Locale: "fr"},
		{Text: "Hallo Welt", Locale: "de"},
	}

	responses, err := p.DetectEntitiesBatch(t.Context(), reqs)
	require.NoError(t, err)
	require.Len(t, responses, 3)

	for i, resp := range responses {
		require.Len(t, resp.Entities, 1, "response %d", i)
		assert.Equal(t, model.EntityPerson, resp.Entities[0].Type)
	}
}

func TestMapEntityType(t *testing.T) {
	tests := []struct {
		azure    string
		expected model.EntityType
	}{
		{"Person", model.EntityPerson},
		{"PersonType", model.EntityPerson},
		{"Organization", model.EntityOrganization},
		{"OrganizationMedical", model.EntityOrganization},
		{"Product", model.EntityProduct},
		{"ComputingProduct", model.EntityProduct},
		{"City", model.EntityLocation},
		{"GPE", model.EntityLocation},
		{"CountryRegion", model.EntityLocation},
		{"Date", model.EntityDate},
		{"DateTime", model.EntityDate},
		{"Time", model.EntityTime},
		{"TimeRange", model.EntityTime},
		{"Currency", model.EntityCurrency},
		{"Age", model.EntityMeasurement},
		{"Temperature", model.EntityMeasurement},
		{"Weight", model.EntityMeasurement},
		{"Email", model.EntityOther},
		{"URL", model.EntityOther},
		{"UnknownType", model.EntityOther},
	}

	for _, tt := range tests {
		t.Run(tt.azure, func(t *testing.T) {
			assert.Equal(t, tt.expected, mapEntityType(tt.azure))
		})
	}
}

func TestLocaleToLanguage(t *testing.T) {
	assert.Equal(t, "en", localeToLanguage("en-US"))
	assert.Equal(t, "fr", localeToLanguage("fr-FR"))
	assert.Equal(t, "de", localeToLanguage("de"))
	assert.Equal(t, "zh", localeToLanguage("zh-Hans-CN"))
}
