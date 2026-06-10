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
		Tags:        []string{"security", "redaction"},
		Cardinality: schema.Monolingual,
		// Source-transform: consumes the in-process secret annotation left by
		// redact and restores originals into both source and target runs.
		Consumes: []schema.IOPort{schema.Port(redaction.SecretAnnotationKey, model.SideSource)},
		Produces: []schema.IOPort{
			schema.Port(schema.PortSource, model.SideSource),
			schema.Port(schema.PortTarget, model.SideTarget),
		},
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
	// Transform producer (AD-006): unredact returns the restored source and
	// target runs as an edit plan — restore is a structured rewrite (each
	// placeholder restore is a known insertion), so the applier rebases
	// surviving source overlays across it. The framework applier rewrites the
	// block; the in-process secret annotation is removed so nothing sensitive
	// reaches the writer.
	base.Transform = t.handleBlock
	t.BaseTool = base
	return t, nil
}

func (t *UnredactTool) handleBlock(v tool.BlockView) (tool.EditPlan, error) {
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
		return tool.EditPlan{}, nil
	}

	var plan tool.EditPlan
	// Restore by placeholder ID (structure-preserving formats: in-process
	// pipelines, XLIFF inline codes) and by visible token text (formats that
	// flattened the placeholder to its string on write), in one pass that also
	// yields the structured edits for overlay rebasing.
	if sr, n, edits := redaction.RestorePlan(v.SourceRuns(), get, entries); n > 0 {
		plan.NewRuns = sr
		plan.Edits = edits
	}
	for _, locale := range v.TargetLocales() {
		if tr, n, _ := redaction.RestorePlan(v.TargetRuns(locale), get, entries); n > 0 {
			plan.SetTarget(locale, tr)
		}
	}

	v.RemoveAnnotation(redaction.SecretAnnotationKey)
	return plan, nil
}
