package tools

import (
	"context"
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
		// Restoring the originals is the point, so the exec run needs an -o for
		// the same reason redact does (#1476).
		WritesOutput: true,
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

	var read *redaction.SecretAnnotation
	if t.vault == nil {
		read, _ = ann.(*redaction.SecretAnnotation)
	}

	var plan tool.EditPlan
	// Restore by placeholder ID (structure-preserving formats: in-process
	// pipelines, XLIFF inline codes) and by visible token text (formats that
	// flattened the placeholder to its string on write), in one pass that also
	// yields the structured edits for overlay rebasing. Plural forms and select
	// cases are restored the same way.
	if sr, n, edits := redaction.RestoreTree(v.SourceRuns(), get, entries); n > 0 {
		// A source nothing changed between redact and unredact goes back as
		// redact read it, its runs unsplit.
		if read != nil && read.Source != nil && redaction.SameText(sr, read.Source) {
			sr = read.Source
		}
		plan.NewRuns = sr
		plan.Edits = edits
	}
	var kept map[model.LocaleID]model.Edition
	for _, locale := range v.TargetLocales() {
		tr, n, _ := redaction.RestoreTree(v.TargetRuns(locale), get, entries)
		if n == 0 {
			continue
		}
		// A translation nothing changed between redact and unredact goes back
		// as redact read it, and keeps its status and origin: the rewrite
		// lands as a tool's draft, so Process stamps them back once it has.
		if read != nil {
			if was, ok := read.Targets[locale]; ok && redaction.SameText(tr, was.Runs) {
				tr = was.Runs
				if kept == nil {
					kept = map[model.LocaleID]model.Edition{}
				}
				kept[locale] = was
			}
		}
		plan.SetTarget(locale, tr)
	}
	if kept != nil {
		v.Annotate(keptProvenanceKey, &keptProvenance{Targets: kept})
	}

	v.RemoveAnnotation(redaction.SecretAnnotationKey)
	return plan, nil
}

// keptProvenanceKey is the annotation under which unredact carries, from its
// handler to the end of its own dispatch, the status and origin of each
// translation it put back as redact read it.
const keptProvenanceKey = "redaction.kept-provenance"

type keptProvenance struct {
	Targets map[model.LocaleID]model.Edition
}

func (*keptProvenance) TypeName() string { return keptProvenanceKey }

// Process runs the restore and stamps back the provenance of each
// translation it put back unchanged.
func (t *UnredactTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	mid := make(chan *model.Part)
	errc := make(chan error, 1)
	go func() {
		defer close(mid)
		errc <- t.BaseTool.Process(ctx, in, mid)
	}()
	for p := range mid {
		keepProvenance(p)
		select {
		case out <- p:
		case <-ctx.Done():
		}
	}
	return <-errc
}

// Apply is ApplyContext with no deadline.
func (t *UnredactTool) Apply(part *model.Part) (*model.Part, error) {
	return t.ApplyContext(context.Background(), part)
}

// ApplyContext restores one part and stamps back the provenance of each
// translation it put back unchanged.
func (t *UnredactTool) ApplyContext(ctx context.Context, part *model.Part) (*model.Part, error) {
	res, err := t.BaseTool.ApplyContext(ctx, part)
	if err == nil {
		keepProvenance(res)
	}
	return res, err
}

// keepProvenance gives each translation unredact put back as redact read it
// the status and origin it had before redact rewrote it.
func keepProvenance(p *model.Part) {
	if p == nil {
		return
	}
	b, ok := p.Resource.(*model.Block)
	if !ok || b == nil {
		return
	}
	kept, ok := model.AnnoAs[*keptProvenance](b, keptProvenanceKey)
	if !ok {
		return
	}
	v := tool.NewVariantView(b)
	for locale, was := range kept.Targets {
		v.StampTargetProvenance(locale, model.TargetStatus(was.Status), was.Origin)
	}
	v.RemoveAnnotation(keptProvenanceKey)
}
