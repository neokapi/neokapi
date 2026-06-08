package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// UnredactConfig configures the unredact tool.
type UnredactConfig struct {
	// VaultPath, when set, reads originals from a sidecar
	// [redaction.FileVault] (external mode). Otherwise unredact restores
	// from the in-process SecretAnnotation left by redact.
	VaultPath string `json:"vaultPath,omitempty" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *UnredactConfig) ToolName() string { return "unredact" }

// Reset restores default values.
func (c *UnredactConfig) Reset() { c.VaultPath = "" }

// Validate checks configuration validity.
func (c *UnredactConfig) Validate() error { return nil }

// UnredactSchema returns the auto-generated schema for the unredact tool.
func UnredactSchema() *schema.ComponentSchema {
	return schema.FromStruct(&UnredactConfig{}, schema.ToolMeta{
		ID:          "unredact",
		Category:    schema.CategoryTextProcessing,
		DisplayName: "Unredact",
		Description: "Restore original values into redacted content after processing",
		Inputs:      []string{schema.PartTypeBlock},
		Tags:        []string{"security", "redaction"},
		Cardinality: schema.Monolingual,
	})
}

// UnredactTool restores original values into redaction placeholders across a
// block's source and target runs, then removes the in-process secret
// annotation so nothing sensitive reaches the writer.
type UnredactTool struct {
	*tool.BaseTool
	cfg   *UnredactConfig
	vault *redaction.FileVault // non-nil in external mode
}

// NewUnredactFromConfig builds an unredact tool from a config map.
func NewUnredactFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg UnredactConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("unredact config: %w", err)
	}
	return NewUnredactTool(&cfg)
}

// NewUnredactTool builds an unredact tool, opening the sidecar vault if
// configured.
func NewUnredactTool(cfg *UnredactConfig) (*UnredactTool, error) {
	t := &UnredactTool{cfg: cfg}
	if cfg.VaultPath != "" {
		v, err := redaction.OpenFileVault(cfg.VaultPath)
		if err != nil {
			return nil, err
		}
		t.vault = v
	}
	base := &tool.BaseTool{
		ToolName:        "unredact",
		ToolDescription: "Restores original values into redacted content after processing",
		Cfg:             cfg,
	}
	// Transform: unredact rewrites both source and target runs to restore originals.
	base.Transform = t.handleBlock
	t.BaseTool = base
	return t, nil
}

func (t *UnredactTool) handleBlock(v tool.SourceView) error {
	// Retrieve the block's annotations to find the secret annotation.
	// We need to use the Annotations() map to find the secret key and delete it.
	annotations := v.Annotations()
	ann, hasAnn := annotations[redaction.SecretAnnotationKey]

	var get func(string) (string, bool)
	var entries []redaction.RedactedValue

	if t.vault != nil {
		blockID := v.ID()
		get = func(token string) (string, bool) {
			val, ok := t.vault.Get(blockID, token)
			return val.Original, ok
		}
		entries = redaction.ValuesForBlock(t.vault, blockID)
	} else if hasAnn {
		if secretAnn, ok := ann.(*redaction.SecretAnnotation); ok {
			get = func(token string) (string, bool) {
				val, ok := secretAnn.Get(token)
				return val.Original, ok
			}
			entries = make([]redaction.RedactedValue, 0, len(secretAnn.Values))
			for _, val := range secretAnn.Values {
				entries = append(entries, val)
			}
		}
	}

	if get == nil {
		return nil
	}

	restore := func(runs []model.Run) ([]model.Run, int) {
		// ID-based restore first (structure-preserving formats: in-process
		// pipelines, XLIFF inline codes), then text-based restore for formats
		// that flattened the placeholder to its visible string on write.
		runs, n1 := redaction.Restore(runs, get)
		runs, n2 := redaction.RestoreText(runs, entries)
		return runs, n1 + n2
	}

	if sr, n := restore(v.SourceRuns()); n > 0 {
		v.SetSourceRuns(sr)
	}
	for _, locale := range v.TargetLocales() {
		if tr, n := restore(v.TargetRuns(locale)); n > 0 {
			v.SetTargetRuns(locale, tr)
		}
	}

	v.RemoveAnnotation(redaction.SecretAnnotationKey)
	return nil
}
