// Package azure implements the NER provider interface using Azure AI Language Services.
//
// Azure NER supports 45+ entity types across 70+ languages via the Text Analytics API.
// This implementation uses the /language/:analyze-text endpoint (API version 2024-11-01).
//
// See Bowrain AD-015 for the hybrid LLM + NER extraction architecture.
package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/model"
)

const (
	apiVersion   = "2024-11-01"
	maxBatchSize = 25   // Azure limit per request
	maxDocChars  = 5120 // Azure character limit per document
)

// Provider implements ner.Provider using Azure AI Language Services.
type Provider struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// New creates a new Azure NER provider.
func New(cfg ner.Config) (*Provider, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("azure ner: endpoint is required")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("azure ner: api_key is required")
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	return &Provider{
		endpoint:   endpoint,
		apiKey:     cfg.APIKey,
		httpClient: &http.Client{},
	}, nil
}

func (p *Provider) Name() string { return "azure" }

func (p *Provider) SupportedLocales() []model.LocaleID {
	return nil // Azure supports 70+ languages; return nil to indicate all.
}

func (p *Provider) Close() error { return nil }

func (p *Provider) DetectEntities(ctx context.Context, req ner.Request) (*ner.Response, error) {
	responses, err := p.DetectEntitiesBatch(ctx, []ner.Request{req})
	if err != nil {
		return nil, err
	}
	if len(responses) == 0 {
		return &ner.Response{}, nil
	}
	return &responses[0], nil
}

func (p *Provider) DetectEntitiesBatch(ctx context.Context, reqs []ner.Request) ([]ner.Response, error) {
	if len(reqs) == 0 {
		return nil, nil
	}

	results := make([]ner.Response, len(reqs))

	// Process in chunks of maxBatchSize.
	for start := 0; start < len(reqs); start += maxBatchSize {
		end := min(start+maxBatchSize, len(reqs))
		chunk := reqs[start:end]

		chunkResults, err := p.doBatch(ctx, chunk)
		if err != nil {
			return nil, fmt.Errorf("azure ner: batch %d-%d: %w", start, end, err)
		}

		copy(results[start:end], chunkResults)
	}

	return results, nil
}

func (p *Provider) doBatch(ctx context.Context, reqs []ner.Request) ([]ner.Response, error) {
	// Build request body.
	docs := make([]document, len(reqs))
	for i, req := range reqs {
		text := req.Text
		if len(text) > maxDocChars {
			text = text[:maxDocChars]
		}
		docs[i] = document{
			ID:       strconv.Itoa(i),
			Language: localeToLanguage(req.Locale),
			Text:     text,
		}
	}

	body := analyzeRequest{
		Kind: "EntityRecognition",
		AnalysisInput: analysisInput{
			Documents: docs,
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/language/:analyze-text?api-version=%s", p.endpoint, apiVersion)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Ocp-Apim-Subscription-Key", p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	var result analyzeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Map results back by document ID.
	results := make([]ner.Response, len(reqs))
	for _, doc := range result.Results.Documents {
		var idx int
		if _, err := fmt.Sscanf(doc.ID, "%d", &idx); err != nil || idx >= len(reqs) {
			continue
		}
		var entities []ner.DetectedEntity
		for _, e := range doc.Entities {
			entityType := mapEntityType(e.Type)
			entities = append(entities, ner.DetectedEntity{
				Text:       e.Text,
				Type:       entityType,
				Confidence: e.ConfidenceScore,
				Offset:     e.Offset,
				Length:     e.Length,
			})
		}
		results[idx] = ner.Response{Entities: entities}
	}

	return results, nil
}

// mapEntityType maps Azure entity types to model.EntityType.
func mapEntityType(azureType string) model.EntityType {
	switch azureType {
	case "Person", "PersonType":
		return model.EntityPerson
	case "Organization", "OrganizationMedical", "OrganizationSports", "OrganizationStockExchange":
		return model.EntityOrganization
	case "Product", "ComputingProduct":
		return model.EntityProduct
	case "Address", "Airport", "City", "Continent", "CountryRegion", "GPE",
		"Geological", "Location", "State", "Structural":
		return model.EntityLocation
	case "Date", "DateTime", "DateRange", "DateTimeRange":
		return model.EntityDate
	case "Time", "TimeRange":
		return model.EntityTime
	case "Currency":
		return model.EntityCurrency
	case "Age", "Area", "Dimension", "Height", "Length", "Number",
		"NumberRange", "Ordinal", "Percentage", "Speed", "Temperature",
		"Volume", "Weight":
		return model.EntityMeasurement
	default:
		return model.EntityOther
	}
}

// localeToLanguage extracts the language code from a locale ID (e.g., "en-US" → "en").
func localeToLanguage(locale model.LocaleID) string {
	s := string(locale)
	if idx := strings.IndexByte(s, '-'); idx > 0 {
		return s[:idx]
	}
	return s
}

// --- Azure API types ---

type analyzeRequest struct {
	Kind          string        `json:"kind"`
	AnalysisInput analysisInput `json:"analysisInput"`
}

type analysisInput struct {
	Documents []document `json:"documents"`
}

type document struct {
	ID       string `json:"id"`
	Language string `json:"language"`
	Text     string `json:"text"`
}

type analyzeResponse struct {
	Results analyzeResults `json:"results"`
}

type analyzeResults struct {
	Documents []documentResult `json:"documents"`
	Errors    []documentError  `json:"errors"`
}

type documentResult struct {
	ID       string        `json:"id"`
	Entities []azureEntity `json:"entities"`
}

type azureEntity struct {
	Text            string  `json:"text"`
	Type            string  `json:"type"`
	Offset          int     `json:"offset"`
	Length          int     `json:"length"`
	ConfidenceScore float64 `json:"confidenceScore"`
}

type documentError struct {
	ID    string `json:"id"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
