package project

import (
	"bytes"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/bowrain/plugin/schema"
	coreproj "github.com/neokapi/neokapi/core/project"
	"gopkg.in/yaml.v3"
)

// ─── Type aliases & re-exports for backward compatibility ─────────────
//
// The schema types and decoders live in bowrain/plugin/schema, which is
// the package the framework's project loader consults at validation time.
// We alias them back here so existing call sites in bowrain CLI commands,
// MCP tools, and the source connector keep compiling without churn.

// ServerSpec captures the optional bowrain-server connection details.
type ServerSpec = schema.ServerSpec

// HooksSpec maps lifecycle trigger names to a list of flow names.
type HooksSpec = schema.HooksSpec

// AutomationSpec defines a single local automation rule.
type AutomationSpec = schema.AutomationSpec

// ActionConfig describes a single action in an automation rule.
type ActionConfig = schema.ActionConfig

// AssetsSpec controls project-wide media asset sync behavior.
type AssetsSpec = schema.AssetsSpec

// BrandVoiceSpec holds brand voice profile bindings for a project.
type BrandVoiceSpec = schema.BrandVoiceSpec

// BrandVoiceEntry is a per-scope brand voice binding.
type BrandVoiceEntry = schema.BrandVoiceEntry

// ProjectURLInfo holds the parts extracted from a compound project URL.
type ProjectURLInfo = schema.ProjectURLInfo

// Hook trigger names. Hooks run synchronously around lifecycle operations.
const (
	HookPrePush  = schema.HookPrePush
	HookPostPush = schema.HookPostPush
	HookPrePull  = schema.HookPrePull
	HookPostPull = schema.HookPostPull
	HookPreFlow  = schema.HookPreFlow
	HookPostFlow = schema.HookPostFlow
)

// Automation action types.
const (
	ActionRunFlow       = schema.ActionRunFlow
	ActionWaitTranslate = schema.ActionWaitTranslate
	ActionPull          = schema.ActionPull
	ActionPush          = schema.ActionPush
)

// Stream constants.
const (
	StreamAuto = schema.StreamAuto
	StreamMain = schema.StreamMain
)

// ParseProjectURL parses a compound project URL into its parts.
func ParseProjectURL(rawURL string) ProjectURLInfo { return schema.ParseProjectURL(rawURL) }

// FormatProjectURL constructs a compound project URL from its parts.
func FormatProjectURL(serverURL, workspace, projectID string) string {
	return schema.FormatProjectURL(serverURL, workspace, projectID)
}

// ─── Recipe ───────────────────────────────────────────────────────────

// Recipe is the bowrain view of a kapi project recipe.
//
// It embeds the framework's KapiProject so consumers can read framework
// fields directly (recipe.Defaults, recipe.Content, recipe.Plugins, ...)
// and adds bowrain-specific extension fields (Server, Hooks, Automations,
// Assets, BrandVoice) at the same YAML top level.
//
// On disk, a Recipe is just a *.kapi file with extra top-level keys. The
// framework loader (coreproj.Load) sees those keys as unknowns and
// captures them in KapiProject.Extras; the bowrain loader (LoadRecipe)
// decodes them into typed fields on Recipe. This keeps the framework
// completely free of bowrain-specific knowledge while allowing both
// loaders to round-trip the same file.
type Recipe struct {
	coreproj.KapiProject `yaml:",inline"`

	Server      *ServerSpec      `yaml:"server,omitempty" json:"server,omitempty"`
	Hooks       HooksSpec        `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Automations []AutomationSpec `yaml:"automations,omitempty" json:"automations,omitempty"`
	Assets      *AssetsSpec      `yaml:"assets,omitempty" json:"assets,omitempty"`
	BrandVoice  *BrandVoiceSpec  `yaml:"brand_voice,omitempty" json:"brand_voice,omitempty"`
}

// LoadRecipe reads a *.kapi file and decodes both framework and bowrain
// fields into a *Recipe.
func LoadRecipe(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recipe: %w", err)
	}
	var r Recipe
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse recipe: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("invalid recipe: %w", err)
	}
	return &r, nil
}

// FindRecipe walks up from start, finds a *.kapi recipe, and decodes it
// as a bowrain Recipe. Mirrors coreproj.FindProject.
func FindRecipe(start string) (*Recipe, coreproj.Layout, error) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, coreproj.Layout{}, fmt.Errorf("recipe: get cwd: %w", err)
		}
		start = cwd
	}
	var layout coreproj.Layout
	if info, err := os.Stat(start); err == nil && !info.IsDir() {
		l, err := coreproj.LayoutFor(start)
		if err != nil {
			return nil, coreproj.Layout{}, err
		}
		layout = l
	} else {
		l, err := coreproj.ResolveLayout(start)
		if err != nil {
			return nil, coreproj.Layout{}, err
		}
		layout = l
	}
	r, err := LoadRecipe(layout.RecipePath)
	if err != nil {
		return nil, coreproj.Layout{}, err
	}
	return r, layout, nil
}

// SaveRecipe writes the recipe to path, encoding both framework and
// bowrain fields. Atomic via temp file + rename.
func SaveRecipe(path string, r *Recipe) error {
	if r.Version == "" {
		r.Version = coreproj.CurrentVersion
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(r); err != nil {
		return fmt.Errorf("marshal recipe: %w", err)
	}
	_ = enc.Close()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write recipe: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename recipe: %w", err)
	}
	return nil
}

// Save persists this recipe back to its source path. The caller is
// responsible for tracking the path; use SaveRecipe directly if you have
// the path already.
func (r *Recipe) Save(path string) error {
	return SaveRecipe(path, r)
}

// Validate delegates to the framework loader. The schema decoders
// registered by bowrain/plugin/schema validate the bowrain-specific
// extension blocks during KapiProject.Validate, so this method has
// nothing left to add — it stays as a hook for callers that explicitly
// validate after mutating a Recipe.
func (r *Recipe) Validate() error {
	if err := r.KapiProject.Validate(); err != nil {
		return err
	}
	if err := r.Server.Validate(); err != nil {
		return fmt.Errorf("server.%w", err)
	}
	if err := r.Hooks.Validate(); err != nil {
		return fmt.Errorf("hooks: %w", err)
	}
	for i, auto := range r.Automations {
		if err := auto.Validate(); err != nil {
			if auto.Name != "" {
				return fmt.Errorf("automations[%d] (%q): %w", i, auto.Name, err)
			}
			return fmt.Errorf("automations[%d]: %w", i, err)
		}
	}
	if err := r.Assets.Validate(); err != nil {
		return fmt.Errorf("assets: %w", err)
	}
	if err := r.BrandVoice.Validate(); err != nil {
		return fmt.Errorf("brand_voice: %w", err)
	}
	return nil
}

// HasServer reports whether the recipe declares a bowrain-server
// connection.
func (r *Recipe) HasServer() bool {
	return r.Server != nil && r.Server.URL != ""
}

// AssetsEnabled reports whether asset sync is enabled. Defaults to true
// when no Assets block is declared.
func (r *Recipe) AssetsEnabled() bool {
	return r.Assets.IsEnabled()
}

// DefaultCollection returns the recipe-level default server-side
// collection routing, decoded from defaults.collection (a bowrain
// extension stored under Defaults.Extras).
func (r *Recipe) DefaultCollection() string {
	if r.Defaults.Extras == nil {
		return ""
	}
	node, ok := r.Defaults.Extras["collection"]
	if !ok {
		return ""
	}
	var s string
	_ = node.Decode(&s)
	return s
}

// SetDefaultCollection writes the project-level default collection.
// Empty string clears the field.
func (r *Recipe) SetDefaultCollection(name string) error {
	if name == "" {
		delete(r.Defaults.Extras, "collection")
		return nil
	}
	if r.Defaults.Extras == nil {
		r.Defaults.Extras = map[string]yaml.Node{}
	}
	var node yaml.Node
	if err := node.Encode(name); err != nil {
		return err
	}
	r.Defaults.Extras["collection"] = node
	return nil
}

// ContentItemView pairs a framework ContentItem with bowrain-specific
// per-item extension fields decoded from the item's Extras map.
type ContentItemView struct {
	coreproj.ContentItem

	Collection   string
	Base         string
	Assets       *bool
	AssetMaxSize string
}

// IteratedItem is the bowrain-aware variant of coreproj.IteratedItem,
// pairing the parent collection with a ContentItemView that carries the
// per-item bowrain extensions.
type IteratedItem struct {
	Collection *coreproj.ContentCollection
	Item       ContentItemView
}

// IterateContent walks all content collections in the recipe and yields
// each item with its decoded bowrain-specific per-item fields.
func (r *Recipe) IterateContent() []IteratedItem {
	base := r.KapiProject.IterateContent()
	out := make([]IteratedItem, 0, len(base))
	for _, it := range base {
		view := ContentItemView{ContentItem: it.Item}
		decodeStringExtra(it.Item.Extras, "collection", &view.Collection)
		decodeStringExtra(it.Item.Extras, "base", &view.Base)
		decodeBoolPtrExtra(it.Item.Extras, "assets", &view.Assets)
		decodeStringExtra(it.Item.Extras, "asset_max_size", &view.AssetMaxSize)
		out = append(out, IteratedItem{Collection: it.Collection, Item: view})
	}
	return out
}

// ResolvedCollection returns the routing collection for this item,
// falling back to the parent named collection and then to the recipe's
// DefaultCollection.
func (item *ContentItemView) ResolvedCollection(coll *coreproj.ContentCollection, recipe *Recipe) string {
	if item.Collection != "" {
		return item.Collection
	}
	if coll != nil && coll.Name != "" && len(coll.Items) > 0 {
		return coll.Name
	}
	return recipe.DefaultCollection()
}

// AddContentItem appends a bare-entry ContentCollection to the recipe,
// applying any non-empty bowrain per-item fields via the entry's Extras
// map. Used by `bowrain add`.
func (r *Recipe) AddContentItem(path string, format *coreproj.FormatSpec, target string, view ContentItemView) {
	entry := coreproj.ContentCollection{
		Path:   path,
		Format: format,
		Target: target,
	}
	if view.Collection != "" {
		setEntryExtra(&entry, "collection", view.Collection)
	}
	if view.Base != "" {
		setEntryExtra(&entry, "base", view.Base)
	}
	if view.Assets != nil {
		setEntryExtra(&entry, "assets", *view.Assets)
	}
	if view.AssetMaxSize != "" {
		setEntryExtra(&entry, "asset_max_size", view.AssetMaxSize)
	}
	r.Content = append(r.Content, entry)
}

// ─── extras helpers ─────────────────────────────────────────────────

func decodeStringExtra(extras map[string]yaml.Node, key string, target *string) {
	if extras == nil {
		return
	}
	if node, ok := extras[key]; ok {
		_ = node.Decode(target)
	}
}

func decodeBoolPtrExtra(extras map[string]yaml.Node, key string, target **bool) {
	if extras == nil {
		return
	}
	node, ok := extras[key]
	if !ok {
		return
	}
	var v bool
	if err := node.Decode(&v); err == nil {
		*target = &v
	}
}

func setEntryExtra(entry *coreproj.ContentCollection, key string, value any) {
	if entry.Extras == nil {
		entry.Extras = map[string]yaml.Node{}
	}
	var node yaml.Node
	if err := node.Encode(value); err == nil {
		entry.Extras[key] = node
	}
}
