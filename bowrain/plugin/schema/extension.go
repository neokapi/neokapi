package schema

import (
	"fmt"

	coreproj "github.com/neokapi/neokapi/core/project"
	"gopkg.in/yaml.v3"
)

// Group is the extension group name for bowrain. Recipes that depend on
// the bowrain platform should declare `requires: [bowrain]` to refuse to
// load when no host has registered the bowrain schema.
const Group = "bowrain"

func init() {
	coreproj.RegisterExtensionGroup(Group, []coreproj.Extension{
		// ── Project-level top-level keys ──────────────────────────
		{Name: "server", Scope: coreproj.ScopeProject, Decoder: serverDecoder},
		{Name: "hooks", Scope: coreproj.ScopeProject, Decoder: hooksDecoder},
		{Name: "automations", Scope: coreproj.ScopeProject, Decoder: automationsDecoder},
		{Name: "assets", Scope: coreproj.ScopeProject, Decoder: assetsDecoder},
		{Name: "brand_voice", Scope: coreproj.ScopeProject, Decoder: brandVoiceDecoder},

		// ── Per-item keys ─────────────────────────────────────────
		{Name: "collection", Scope: coreproj.ScopeItem, Decoder: stringDecoder},
		{Name: "base", Scope: coreproj.ScopeItem, Decoder: stringDecoder},
		{Name: "assets", Scope: coreproj.ScopeItem, Decoder: boolDecoder},
		{Name: "asset_max_size", Scope: coreproj.ScopeItem, Decoder: stringDecoder},

		// ── Defaults-level keys ───────────────────────────────────
		{Name: "collection", Scope: coreproj.ScopeDefaults, Decoder: stringDecoder},

		// ── Named-collection-level keys ───────────────────────────
		{Name: "collection", Scope: coreproj.ScopeCollection, Decoder: stringDecoder},
	})
}

// serverDecoder validates the top-level `server:` block.
var serverDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var s ServerSpec
	if err := n.Decode(&s); err != nil {
		return fmt.Errorf("decode server: %w", err)
	}
	return s.Validate()
})

// hooksDecoder validates the top-level `hooks:` block.
var hooksDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var h HooksSpec
	if err := n.Decode(&h); err != nil {
		return fmt.Errorf("decode hooks: %w", err)
	}
	return h.Validate()
})

// automationsDecoder validates the top-level `automations:` block.
var automationsDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var auto []AutomationSpec
	if err := n.Decode(&auto); err != nil {
		return fmt.Errorf("decode automations: %w", err)
	}
	for i, a := range auto {
		if err := a.Validate(); err != nil {
			if a.Name != "" {
				return fmt.Errorf("[%d] (%q): %w", i, a.Name, err)
			}
			return fmt.Errorf("[%d]: %w", i, err)
		}
	}
	return nil
})

// assetsDecoder validates the top-level `assets:` block.
var assetsDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var a AssetsSpec
	if err := n.Decode(&a); err != nil {
		return fmt.Errorf("decode assets: %w", err)
	}
	return a.Validate()
})

// brandVoiceDecoder validates the top-level `brand_voice:` block.
var brandVoiceDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var bv BrandVoiceSpec
	if err := n.Decode(&bv); err != nil {
		return fmt.Errorf("decode brand_voice: %w", err)
	}
	return bv.Validate()
})

// stringDecoder accepts any scalar string and rejects non-string nodes.
var stringDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var s string
	if err := n.Decode(&s); err != nil {
		return fmt.Errorf("expected string: %w", err)
	}
	return nil
})

// boolDecoder accepts any scalar bool and rejects non-bool nodes.
var boolDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var b bool
	if err := n.Decode(&b); err != nil {
		return fmt.Errorf("expected bool: %w", err)
	}
	return nil
})

// Static check that the decoder type satisfies the framework interface.
var _ coreproj.ExtensionDecoder = stringDecoder
