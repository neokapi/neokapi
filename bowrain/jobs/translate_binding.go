package jobs

import (
	"context"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/ai/tools"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coreproject "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	"github.com/neokapi/neokapi/terms"
)

// TranslateScopeStore is what the binding reads: the project, stream and
// collection the voice ladder walks, plus the item whose collection places the
// content. store.ContentStore satisfies it.
type TranslateScopeStore interface {
	voicescope.ScopeStore
	GetItem(ctx context.Context, projectID, stream, itemName string) (*store.Item, error)
}

// TranslateBinding is where a server-side AI translation reads its governing
// context from.
//
// One struct for every server-side surface: the interactive editor translate,
// the worker's queued jobs, and the model sweep that measures under the context
// those jobs run with. A translation started from a keystroke and one started
// from the queue carry the same content under the same governance, so both read
// it from here.
//
// Every field is optional. A zero binding produces a bare config, and a store
// that cannot answer leaves its field unset rather than failing the
// translation: the governing context is advisory, and the checks still report
// what the model got wrong.
type TranslateBinding struct {
	// Store reads the project, stream, collection and item.
	Store TranslateScopeStore
	// Voice resolves a bound voice profile id. Nil translates without voice.
	Voice coreprofile.Store
	// WorkspaceDefault is the base rung of the voice ladder: the workspace's
	// own default profile. Nil skips that rung.
	WorkspaceDefault voicescope.WorkspaceDefault
	// Terms holds the concepts the per-locale term rules derive from. Nil
	// translates without terminology.
	Terms terms.Terminology
	// Memory is the workspace content memory. It answers what a block said
	// before, so an edited source carries its settled wording into the prompt
	// instead of being translated afresh. Nil translates without that history.
	Memory memory.ContentMemory

	// Project supplies the source locale and the do-not-translate terms.
	Project *store.Project

	WorkspaceID string
	ProjectID   string
	// Stream scopes the voice ladder and the item read. Empty means "main".
	Stream string
	// ItemName is the item being translated. It names the collection, which is
	// what places the content in the project's context space.
	ItemName string

	TargetLocale     model.LocaleID
	BatchSize        int
	BatchConcurrency int
}

// BuildTranslateConfig assembles the AI translate tool config for a server-side
// translation, binding everything that governs the run:
//
//   - the voice profile resolved through the platform's ladder (collection →
//     stream → project → workspace default), rendered into every prompt;
//   - the per-locale term rules from the workspace terms, so the model is told
//     the mandated renderings at generation time rather than term-check
//     flagging them afterwards;
//   - the do-not-translate terms, which are enforced by masking rather than
//     asked for;
//   - the content memory and the point, which together let a block be offered
//     its own previously approved translation, read from the place it belongs
//     to rather than from wherever the corpus answers first;
//   - the surrounding blocks, so a bare "Save" is not a coin flip between a
//     verb and a noun.
func BuildTranslateConfig(ctx context.Context, b TranslateBinding) tools.AITranslateConfig {
	var source model.LocaleID
	if b.Project != nil {
		source = b.Project.DefaultSourceLanguage
	}
	col := b.Collection(ctx)

	cfg := tools.AITranslateConfig{
		SourceLocale:     source,
		TargetLocale:     b.TargetLocale,
		BatchSize:        b.BatchSize,
		BatchConcurrency: b.BatchConcurrency,
		Profile:          b.VoiceProfile(ctx, col),
		TermRules:        b.termRules(ctx, source),
		DNT:              ProjectDNTTerms(b.Project),
		// Where this content sits, so the block's history is read from its own
		// place. A wording approved for one collection must not steer another.
		Point: CollectionPoint(col),
		// The block's own previously approved translation, when the corpus
		// holds one and the rules it was approved under still hold.
		Reuse: tools.ReusePrior,
		// The blocks either side, as reference the model reads and does not
		// translate. Both surfaces hand the tool one item's blocks in document
		// order, which is the neighbourhood a reader of the item sees.
		Context:       tools.ContextNeighbours,
		ContextWindow: tools.DefaultContextWindow,
	}
	if provider := b.memoryProvider(); provider != nil {
		cfg.Memory = provider
	}
	return cfg
}

// CollectionPoint renders where a collection's content sits, as the content
// memory records it: the product, the channel it ships on, and the collection
// itself, coarsest first.
//
// The three rungs are the ones kapi writes beside every answer it learns from a
// committed translation, so a lookup made from the platform names the same
// place a lookup made from the CLI does. The structural axes travel on the push
// wire in the collection's coordinates (core/project: a profile IS the product
// value, a channel IS the channel value); the channel falls back to the key
// core/profile reads on the collection for a row written before the coordinates
// were declared. A collection that declares nothing renders as the empty point,
// which is the project's default place rather than a missing one.
func CollectionPoint(col *store.Collection) string {
	if col == nil {
		return ""
	}
	channel := col.Context[coreproject.ChannelAxis]
	if channel == "" {
		channel = col.ConnectorConfig[coreprofile.PropertyChannel]
	}
	return memory.NewPoint(col.Context[coreproject.ProductAxis], channel, col.Name)
}

// stream is the stream this binding reads, defaulting to main.
func (b TranslateBinding) stream() string {
	if b.Stream == "" {
		return "main"
	}
	return b.Stream
}

// Collection returns the collection the item belongs to, or nil when the item
// names none, is not stored, or cannot be read. A translation never fails
// because its place could not be resolved: it runs at the project's default
// point instead.
func (b TranslateBinding) Collection(ctx context.Context) *store.Collection {
	if b.Store == nil || b.ProjectID == "" || b.ItemName == "" {
		return nil
	}
	item, err := b.Store.GetItem(ctx, b.ProjectID, b.stream(), b.ItemName)
	if err != nil || item == nil || item.CollectionID == "" {
		return nil
	}
	col, err := b.Store.GetCollection(ctx, b.ProjectID, item.CollectionID)
	if err != nil {
		return nil
	}
	return col
}

// VoiceProfile resolves the voice profile this translation carries, through the
// platform's hierarchical ladder scoped to the item's own collection and the
// request's stream. The target locale selects the profile's locale override, so
// a per-locale formality adjustment reaches the prompt.
//
// Returns nil (and logs) on any resolution failure: voice must never fail a
// translation.
func (b TranslateBinding) VoiceProfile(ctx context.Context, col *store.Collection) *coreprofile.VoiceProfile {
	if b.Voice == nil {
		return nil
	}
	scope := voicescope.Scope{
		WorkspaceID: b.WorkspaceID,
		ProjectID:   b.ProjectID,
		Stream:      b.stream(),
		Locale:      b.TargetLocale,
	}
	if col != nil {
		scope.CollectionID = col.ID
	}
	profile, err := voicescope.Resolve(ctx, b.Store, b.WorkspaceDefault, b.Voice, scope)
	if err != nil {
		slog.WarnContext(ctx, "voice profile resolution failed; translating without voice",
			"project_id", b.ProjectID, "error", err)
		return nil
	}
	return profile
}

// termRules builds the terminology governing this translation from the bound
// terms, via the derivation every server-side surface shares.
//
// Returns nil (and logs) when the terms cannot be read: terminology must never
// fail a translation.
func (b TranslateBinding) termRules(ctx context.Context, source model.LocaleID) []coreprofile.TermRule {
	if b.Terms == nil || source == "" || b.TargetLocale == "" {
		return nil
	}
	rules, err := TermRulesFromConcepts(ctx, b.Terms, b.ProjectID, source, b.TargetLocale)
	if err != nil {
		slog.WarnContext(ctx, "terms read failed; translating without terminology",
			"project_id", b.ProjectID, "error", err)
		return nil
	}
	return rules
}

// memoryProvider wraps the content memory as the provider the framework's
// producers ask, or nil when the run has none.
func (b TranslateBinding) memoryProvider() corememory.Provider {
	if b.Memory == nil {
		return nil
	}
	return leverage.NewProvider(b.Memory)
}
