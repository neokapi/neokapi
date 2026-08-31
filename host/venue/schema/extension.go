package schema

import (
	"fmt"

	coreproj "github.com/neokapi/neokapi/core/project"
	"gopkg.in/yaml.v3"
)

// Group is the extension group name for bowrain. Recipes that depend on
// the bowrain platform should declare `requires: { bowrain: "^1.0" }` to refuse to
// load when no host has registered the bowrain schema.
const Group = "bowrain"

// VenueKey is the recipe key that connects a project to a bowrain server: the
// url, the stream and the convergence policy. It is the platform's own name
// because the block is the platform — the framework finds it through the venue
// flag on the registration, never by spelling it out.
const VenueKey = "bowrain"

func init() {
	coreproj.RegisterExtensionGroup(Group, []coreproj.Extension{
		// ── Project-level top-level keys ──────────────────────────
		// `bowrain:` is the connection itself — the venue key kapi reads to
		// decide where the loop runs. Everything else only means something
		// inside that relationship (automations fire on push and pull,
		// `assets:` governs media asset sync, `hooks:` and `brand_voice:` are
		// validated here and carried on the recipe), so each declares
		// DependsOn: VenueKey — set without it, the field is inert and
		// kapi status/check surface that instead of silently ignoring it.
		{Name: VenueKey, Scope: coreproj.ScopeProject, Decoder: serverDecoder, Venue: true},
		{Name: "hooks", Scope: coreproj.ScopeProject, Decoder: hooksDecoder, DependsOn: VenueKey},
		{Name: "automations", Scope: coreproj.ScopeProject, Decoder: automationsDecoder, DependsOn: VenueKey},
		{Name: "assets", Scope: coreproj.ScopeProject, Decoder: assetsDecoder, DependsOn: VenueKey},
		{Name: "brand_voice", Scope: coreproj.ScopeProject, Decoder: voiceDecoder, DependsOn: VenueKey},

		// ── Per-item keys ─────────────────────────────────────────
		// No `base` here: it is a framework field on ContentItem, so the key
		// is decoded into the struct and never reaches Extras.
		{Name: "collection", Scope: coreproj.ScopeItem, Decoder: stringDecoder},
		{Name: "assets", Scope: coreproj.ScopeItem, Decoder: boolDecoder},
		{Name: "asset_max_size", Scope: coreproj.ScopeItem, Decoder: stringDecoder},

		// ── Defaults-level keys ───────────────────────────────────
		{Name: "collection", Scope: coreproj.ScopeDefaults, Decoder: stringDecoder},

		// ── Named-collection-level keys ───────────────────────────
		{Name: "collection", Scope: coreproj.ScopeCollection, Decoder: stringDecoder},
		// Where this collection's strings can be read in place. Per collection
		// because a repository publishes one host per surface it ships, and
		// DependsOn the venue because a preview is something a reviewer on the
		// server offers — declared without one, it would be inert.
		{Name: "preview", Scope: coreproj.ScopeCollection, Decoder: previewDecoder, DependsOn: VenueKey},
	})
}

// serverDecoder validates the top-level `bowrain:` block.
var serverDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var s ServerSpec
	if err := n.Decode(&s); err != nil {
		return fmt.Errorf("decode %s: %w", VenueKey, err)
	}
	return s.Validate()
})

// previewDecoder validates a collection's `preview:` block.
var previewDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var p PreviewSpec
	if err := n.Decode(&p); err != nil {
		return fmt.Errorf("decode preview: %w", err)
	}
	return p.Validate()
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

// voiceDecoder validates the top-level `brand_voice:` block.
var voiceDecoder = coreproj.ExtensionDecoderFunc(func(n yaml.Node) error {
	var bv VoiceSpec
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
