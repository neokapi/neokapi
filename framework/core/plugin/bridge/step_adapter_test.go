package bridge

import (
	"testing"

	"github.com/neokapi/neokapi/core/schema"
)

func TestBridgeStepToolName(t *testing.T) {
	tool := NewBridgeStepTool(
		nil, BridgeConfig{},
		"net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep",
		"okapi:search-and-replace",
		"Search and Replace",
		nil,
	)

	if got := tool.Name(); got != "okapi:search-and-replace" {
		t.Errorf("Name() = %q, want %q", got, "okapi:search-and-replace")
	}
	if got := tool.Description(); got != "Search and Replace" {
		t.Errorf("Description() = %q, want %q", got, "Search and Replace")
	}
}

func TestBridgeStepToolSchema(t *testing.T) {
	s := &schema.ComponentSchema{
		ID:    "okapi:search-and-replace",
		Title: "Search And Replace",
		Type:  "object",
		Meta: schema.ComponentMeta{
			ID:          "okapi:search-and-replace",
			Type:        "step",
			DisplayName: "Search And Replace",
		},
	}

	tool := NewBridgeStepTool(
		nil, BridgeConfig{},
		"net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep",
		"okapi:search-and-replace",
		"Search and Replace",
		s,
	)

	got := tool.Schema()
	if got == nil {
		t.Fatal("Schema() returned nil")
	}
	if got.ID != "okapi:search-and-replace" {
		t.Errorf("Schema().ID = %q, want %q", got.ID, "okapi:search-and-replace")
	}
	if got.Meta.Type != "step" {
		t.Errorf("Schema().Meta.Type = %q, want %q", got.Meta.Type, "step")
	}
}

func TestBridgeStepToolSetLocales(t *testing.T) {
	tool := NewBridgeStepTool(
		nil, BridgeConfig{},
		"net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep",
		"okapi:search-and-replace",
		"Search and Replace",
		nil,
	)

	tool.SetLocales("en-US", "fr-FR")
	if tool.sourceLocale != "en-US" {
		t.Errorf("sourceLocale = %q, want %q", tool.sourceLocale, "en-US")
	}
	if tool.targetLocale != "fr-FR" {
		t.Errorf("targetLocale = %q, want %q", tool.targetLocale, "fr-FR")
	}
}

func TestBridgeStepToolSetStepParams(t *testing.T) {
	tool := NewBridgeStepTool(
		nil, BridgeConfig{},
		"net.sf.okapi.steps.searchandreplace.SearchAndReplaceStep",
		"okapi:search-and-replace",
		"Search and Replace",
		nil,
	)

	params := map[string]any{
		"search":  "hello",
		"replace": "world",
		"regex":   true,
	}
	tool.SetStepParams(params)

	if tool.stepParams["search"] != "hello" {
		t.Errorf("stepParams[search] = %v, want %q", tool.stepParams["search"], "hello")
	}
	if tool.stepParams["regex"] != true {
		t.Errorf("stepParams[regex] = %v, want true", tool.stepParams["regex"])
	}
}
