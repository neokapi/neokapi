package knowledge

import (
	"context"
	"fmt"
	"strings"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// DefaultMaxSamples caps the number of inspectable sample blocks a blast radius
// or concept-usage report carries when EvalOptions.MaxSamples is not set.
const DefaultMaxSamples = 20

// sampleTextLimit caps the rune length of a sample block's quoted text so a
// report stays light to transfer and render.
const sampleTextLimit = 500

// EvalOptions tunes a blast-radius or concept-usage walk.
type EvalOptions struct {
	// PilotStreams adds streams (beyond the always-included "main") to the walk,
	// keyed by project ID. A change-set's pilots map naturally onto this so a
	// preview can include the content already exercising the draft.
	PilotStreams map[string][]string

	// Locales restricts which locales each block is evaluated in. When empty,
	// each block is evaluated in its own source locale — the canonical, on-brand
	// source text — which is what brand vocabulary and terminology enforcement
	// match against. When set, each block is evaluated in every listed locale
	// for which it has text (Block.Text): the source locale uses the source
	// runs, a target locale uses that target's runs.
	Locales []model.LocaleID

	// MaxSamples caps the number of sample blocks collected (0 →
	// DefaultMaxSamples).
	MaxSamples int
}

func (o EvalOptions) maxSamples() int {
	if o.MaxSamples > 0 {
		return o.MaxSamples
	}
	return DefaultMaxSamples
}

// ---------------------------------------------------------------------------
// Report types
// ---------------------------------------------------------------------------

// ChangeSetImpact is the blast radius of a change-set over stored content: how
// many blocks across the workspace would be newly flagged or resolved if the
// draft merged, broken down per project → collection → (stream, locale), with a
// word count as a proxy for re-translation effort and a capped sample of
// affected blocks for inspection (AD-021).
//
// An evaluation unit is a (block, locale) pair: a block flagged in three target
// locales counts three affected blocks and contributes its source word count
// three times, since each locale is a separate re-translation. The per-level
// counts therefore sum exactly to the totals.
type ChangeSetImpact struct {
	TotalBlocks    int             `json:"total_blocks"`    // (block, locale) rows scanned
	AffectedBlocks int             `json:"affected_blocks"` // rows whose flags or guidance changed
	NewViolations  int             `json:"new_violations"`  // forbidden terms / vocabulary hits the draft raises
	Resolved       int             `json:"resolved"`        // violations the draft clears
	Words          int             `json:"words"`           // source words of affected rows (effort proxy)
	Projects       []ProjectImpact `json:"projects"`
	Samples        []BlockSample   `json:"samples"`
}

// ProjectImpact is the per-project slice of a ChangeSetImpact.
type ProjectImpact struct {
	ProjectID      string             `json:"project_id"`
	ProjectName    string             `json:"project_name"`
	AffectedBlocks int                `json:"affected_blocks"`
	NewViolations  int                `json:"new_violations"`
	Resolved       int                `json:"resolved"`
	Words          int                `json:"words"`
	Collections    []CollectionImpact `json:"collections"`
}

// CollectionImpact is the per-collection slice of a ProjectImpact. When the
// block's item could not be resolved to a collection, CollectionID is empty and
// CollectionName is the item name (blocks group by item).
type CollectionImpact struct {
	CollectionID   string         `json:"collection_id"`
	CollectionName string         `json:"collection_name"`
	AffectedBlocks int            `json:"affected_blocks"`
	NewViolations  int            `json:"new_violations"`
	Resolved       int            `json:"resolved"`
	Words          int            `json:"words"`
	Locales        []LocaleImpact `json:"locales"`
}

// LocaleImpact is the per-(stream, locale) leaf of a CollectionImpact.
type LocaleImpact struct {
	Stream         string         `json:"stream"`
	Locale         model.LocaleID `json:"locale"`
	AffectedBlocks int            `json:"affected_blocks"`
	NewViolations  int            `json:"new_violations"`
	Resolved       int            `json:"resolved"`
	Words          int            `json:"words"`
}

// ConceptUsage is the where-used footprint of a single concept: every stored
// block whose text contains one of the concept's terms, grouped per project →
// collection → (stream, locale). It powers GET /concepts/:cid/blast-radius — the
// "consequences" a steward sees before proposing a change.
type ConceptUsage struct {
	ConceptID   string         `json:"concept_id"`
	TotalBlocks int            `json:"total_blocks"` // (block, locale) rows scanned
	Blocks      int            `json:"blocks"`       // rows containing a term of the concept
	Occurrences int            `json:"occurrences"`  // total term occurrences
	Words       int            `json:"words"`        // source words of rows that contain the concept
	Projects    []ProjectUsage `json:"projects"`
	Samples     []BlockSample  `json:"samples"`
}

// ProjectUsage is the per-project slice of a ConceptUsage.
type ProjectUsage struct {
	ProjectID   string            `json:"project_id"`
	ProjectName string            `json:"project_name"`
	Blocks      int               `json:"blocks"`
	Occurrences int               `json:"occurrences"`
	Words       int               `json:"words"`
	Collections []CollectionUsage `json:"collections"`
}

// CollectionUsage is the per-collection slice of a ProjectUsage.
type CollectionUsage struct {
	CollectionID   string        `json:"collection_id"`
	CollectionName string        `json:"collection_name"`
	Blocks         int           `json:"blocks"`
	Occurrences    int           `json:"occurrences"`
	Words          int           `json:"words"`
	Locales        []LocaleUsage `json:"locales"`
}

// LocaleUsage is the per-(stream, locale) leaf of a CollectionUsage.
type LocaleUsage struct {
	Stream      string         `json:"stream"`
	Locale      model.LocaleID `json:"locale"`
	Blocks      int            `json:"blocks"`
	Occurrences int            `json:"occurrences"`
	Words       int            `json:"words"`
}

// BlockSample is one inspectable affected (or matching) block: enough to locate
// it and show why it surfaced. NewViolations/Resolved are populated for a
// change-set impact; Occurrences is populated for a concept-usage report.
type BlockSample struct {
	ProjectID      string         `json:"project_id"`
	Stream         string         `json:"stream"`
	CollectionID   string         `json:"collection_id"`
	CollectionName string         `json:"collection_name"`
	Locale         model.LocaleID `json:"locale"`
	ItemName       string         `json:"item_name"`
	BlockID        string         `json:"block_id"`
	Text           string         `json:"text"`
	NewViolations  int            `json:"new_violations,omitempty"`
	Resolved       int            `json:"resolved,omitempty"`
	Occurrences    int            `json:"occurrences,omitempty"`
}

// ---------------------------------------------------------------------------
// EvaluateChangeSet — change-set blast radius
// ---------------------------------------------------------------------------

// EvaluateChangeSet computes the blast radius of a change-set's ops over the
// stored content of a workspace, without persisting anything. It walks every
// project's "main" stream (plus any pilot streams named in opts), and for each
// block in each evaluated locale compares the live graph and voice profiles
// (the "before") against the candidates the ops would produce (the "after"):
//
//   - Voice impact reuses core/brand.EvaluateBlastRadius, run per block against
//     each voice profile the change-set's voice ops touch.
//   - Term/concept/relation impact compares the terms the block contains under
//     the before and after termbases: a block is affected when a term it
//     contains gains or loses forbidden status, or when its USE_INSTEAD
//     replacement guidance changes. Newly-forbidden terms count as new
//     violations; no-longer-forbidden terms as resolved.
//
// cs is accepted for symmetry with the persisted lifecycle (and to let callers
// pass a loaded change-set); the ops slice is authoritative for the evaluation.
func (e *Engine) EvaluateChangeSet(ctx context.Context, workspaceID string, cs ChangeSet, ops []ChangeSetOp, opts EvalOptions) (*ChangeSetImpact, error) {
	_ = cs

	// Term side: build the before/after termbase snapshots once.
	before, err := e.buildBeforeTermbase(ctx, ops)
	if err != nil {
		return nil, fmt.Errorf("build before termbase: %w", err)
	}
	after, err := ApplyOpsToTermbase(ctx, before, ops)
	if err != nil {
		return nil, fmt.Errorf("build after termbase: %w", err)
	}

	// Voice side: one (baseline, candidate) pair per profile the voice ops touch.
	pairs, err := e.voicePairs(ctx, ops)
	if err != nil {
		return nil, fmt.Errorf("build voice candidates: %w", err)
	}

	t := newTree(opts.maxSamples())

	walkErr := e.walkBlocks(ctx, workspaceID, opts, func(p *store.Project, stream string, b *store.StoredBlock, locale model.LocaleID, text, colID, colName string) error {
		t.scan()

		vNew, vResolved, vAffected := voiceImpactForBlock(pairs, colID, colName, b.ID, text)
		tNew, tResolved, tAffected, err := termImpact(ctx, before, after, locale, text)
		if err != nil {
			return err
		}

		newV := vNew + tNew
		resolved := vResolved + tResolved
		if !vAffected && !tAffected {
			return nil
		}

		words := b.WordCount()
		t.hit(p, colID, colName, stream, locale, newV, resolved, words, 0, BlockSample{
			ProjectID:      p.ID,
			Stream:         stream,
			CollectionID:   colID,
			CollectionName: colName,
			Locale:         locale,
			ItemName:       b.ItemName,
			BlockID:        b.ID,
			Text:           truncateText(text),
			NewViolations:  newV,
			Resolved:       resolved,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return t.toImpact(), nil
}

// profilePair pairs a baseline voice profile with the candidate the change-set's
// voice ops would produce for it.
type profilePair struct {
	baseline  *corebrand.VoiceProfile
	candidate *corebrand.VoiceProfile
}

// voicePairs loads each profile the change-set's voice ops reference and builds
// its candidate. Profiles that cannot be loaded (absent, or no ProfileStore)
// are skipped so a voice op against a missing profile simply contributes no
// impact rather than failing the whole evaluation.
func (e *Engine) voicePairs(ctx context.Context, ops []ChangeSetOp) ([]profilePair, error) {
	ids := voiceProfileIDs(ops)
	if len(ids) == 0 || e.profiles == nil {
		return nil, nil
	}
	pairs := make([]profilePair, 0, len(ids))
	for _, id := range ids {
		baseline, err := e.profiles.GetProfile(ctx, id)
		if err != nil {
			return nil, err
		}
		if baseline == nil {
			continue
		}
		pairs = append(pairs, profilePair{
			baseline:  baseline,
			candidate: ApplyVoiceOpsToProfile(baseline, ops),
		})
	}
	return pairs, nil
}

// voiceProfileIDs returns the distinct profile IDs the voice ops target, in
// first-seen order.
func voiceProfileIDs(ops []ChangeSetOp) []string {
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, op := range ops {
		switch op.Op {
		case OpVoiceRuleAdd:
			var p VoiceRuleAddPayload
			if decodePayload(op, &p) == nil {
				add(p.ProfileID)
			}
		case OpVoiceRuleRemove:
			var p VoiceRuleRemovePayload
			if decodePayload(op, &p) == nil {
				add(p.ProfileID)
			}
		}
	}
	return ids
}

// voiceImpactForBlock reuses core/brand.EvaluateBlastRadius — the single source
// of brand-vocabulary blast-radius truth — per block against each touched
// profile, summing the new/resolved counts and OR-ing the affected flag.
func voiceImpactForBlock(pairs []profilePair, colID, colName, blockID, text string) (newV, resolved int, affected bool) {
	if len(pairs) == 0 {
		return 0, 0, false
	}
	eb := []corebrand.EvalBlock{{
		BlockID:        blockID,
		CollectionID:   colID,
		CollectionName: colName,
		Text:           text,
	}}
	for _, pr := range pairs {
		br := corebrand.EvaluateBlastRadius(eb, pr.baseline, pr.candidate)
		newV += br.NewViolations
		resolved += br.ResolvedViolations
		if br.AffectedBlocks > 0 {
			affected = true
		}
	}
	return newV, resolved, affected
}

// ---------------------------------------------------------------------------
// Term impact (before vs after termbase)
// ---------------------------------------------------------------------------

// termSig is the resolution signature of a term occurrence: its lifecycle status
// and the preferred replacement its concept's USE_INSTEAD relation points to (if
// any). A change to either is what makes a block "affected" on the term side.
type termSig struct {
	status      model.TermStatus
	replacement string
}

// termImpact compares the terms a block's text contains under the before and
// after termbases. newV counts terms that become forbidden (present and newly
// forbidden); resolved counts terms that stop being forbidden (or are removed);
// changed reports whether any contained term's status or replacement guidance
// differs at all.
func termImpact(ctx context.Context, before, after *termbase.InMemoryTermBase, locale model.LocaleID, text string) (newV, resolved int, changed bool, err error) {
	beforeSig, err := termSignatures(ctx, before, locale, text)
	if err != nil {
		return 0, 0, false, err
	}
	afterSig, err := termSignatures(ctx, after, locale, text)
	if err != nil {
		return 0, 0, false, err
	}

	for k, sa := range afterSig {
		sb, ok := beforeSig[k]
		if sa.status == model.TermForbidden && (!ok || sb.status != model.TermForbidden) {
			newV++
		}
	}
	for k, sb := range beforeSig {
		sa, ok := afterSig[k]
		if sb.status == model.TermForbidden && (!ok || sa.status != model.TermForbidden) {
			resolved++
		}
	}
	return newV, resolved, !sigMapsEqual(beforeSig, afterSig), nil
}

// termSignatures returns the resolution signature of every term of the lookup
// locale that occurs in text, keyed by concept ID + lowered term text so the
// same designation is comparable across the before and after snapshots.
func termSignatures(ctx context.Context, tb *termbase.InMemoryTermBase, locale model.LocaleID, text string) (map[string]termSig, error) {
	out := map[string]termSig{}
	if tb == nil {
		return out, nil
	}
	matches, err := tb.LookupAll(ctx, text, termbase.LookupOptions{SourceLocale: locale})
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		key := m.Concept.ID + "|" + strings.ToLower(m.Term.Text)
		out[key] = termSig{
			status:      m.Term.Status,
			replacement: resolveReplacement(ctx, tb, m.Concept, locale),
		}
	}
	return out, nil
}

// resolveReplacement returns the preferred term (in locale) of the concept that
// c's USE_INSTEAD relation points to — the recommended substitute a steward
// would see — or "" when c has no such guidance.
func resolveReplacement(ctx context.Context, tb *termbase.InMemoryTermBase, c termbase.Concept, locale model.LocaleID) string {
	rels, err := tb.RelationsOf(ctx, c.ID, nil)
	if err != nil {
		return ""
	}
	for _, r := range rels {
		if r.RelationType != graph.LabelUseInstead || r.SourceID != c.ID {
			continue
		}
		target, ok, err := tb.GetConcept(ctx, r.TargetID)
		if err != nil || !ok {
			continue
		}
		if pt := target.PreferredTerm(locale); pt != nil {
			return pt.Text
		}
	}
	return ""
}

// sigMapsEqual reports whether two signature maps describe the same set of
// contained terms with the same status and replacement guidance.
func sigMapsEqual(a, b map[string]termSig) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || va != vb {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// ConceptUsage — concept where-used
// ---------------------------------------------------------------------------

// ConceptUsage walks the workspace's stored content and reports every block
// whose text contains one of the concept's terms, grouped per project →
// collection → (stream, locale). It is the same walk as EvaluateChangeSet
// filtered to one concept's terms, without a candidate side — the "where used"
// behind GET /concepts/:cid/blast-radius.
func (e *Engine) ConceptUsage(ctx context.Context, workspaceID, conceptID string, opts EvalOptions) (*ConceptUsage, error) {
	// A single-concept termbase so LookupAll matches only this concept's terms.
	cTB := termbase.NewInMemoryTermBase()
	if e.concepts != nil {
		if c, ok, err := e.concepts.GetConcept(ctx, conceptID); err != nil {
			return nil, fmt.Errorf("load concept %q: %w", conceptID, err)
		} else if ok {
			if err := cTB.AddConcept(ctx, deepCopyConcept(c)); err != nil {
				return nil, err
			}
		}
	}

	t := newTree(opts.maxSamples())

	walkErr := e.walkBlocks(ctx, workspaceID, opts, func(p *store.Project, stream string, b *store.StoredBlock, locale model.LocaleID, text, colID, colName string) error {
		t.scan()

		matches, err := cTB.LookupAll(ctx, text, termbase.LookupOptions{SourceLocale: locale})
		if err != nil {
			return err
		}
		occ := len(matches)
		if occ == 0 {
			return nil
		}

		words := b.WordCount()
		t.hit(p, colID, colName, stream, locale, 0, 0, words, occ, BlockSample{
			ProjectID:      p.ID,
			Stream:         stream,
			CollectionID:   colID,
			CollectionName: colName,
			Locale:         locale,
			ItemName:       b.ItemName,
			BlockID:        b.ID,
			Text:           truncateText(text),
			Occurrences:    occ,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return t.toUsage(conceptID), nil
}

// ---------------------------------------------------------------------------
// Block walk (shared by EvaluateChangeSet and ConceptUsage)
// ---------------------------------------------------------------------------

// walkBlocks visits every (project, stream, block, locale) evaluation unit in
// the workspace with non-empty text, resolving each block's collection. It
// always walks each project's "main" stream and adds the pilot streams named in
// opts. visit errors abort the walk.
func (e *Engine) walkBlocks(
	ctx context.Context,
	workspaceID string,
	opts EvalOptions,
	visit func(p *store.Project, stream string, b *store.StoredBlock, locale model.LocaleID, text, colID, colName string) error,
) error {
	projects, err := e.blocks.ListProjects(ctx)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}
	for _, p := range projects {
		if workspaceID != "" && p.WorkspaceID != workspaceID {
			continue
		}
		for _, stream := range e.resolveStreams(ctx, p.ID, opts) {
			blocks, err := e.blocks.GetBlocks(ctx, store.BlockQuery{ProjectID: p.ID, Stream: stream})
			if err != nil {
				return fmt.Errorf("get blocks (project %q, stream %q): %w", p.ID, stream, err)
			}
			for _, b := range blocks {
				if b == nil || b.Block == nil {
					continue
				}
				colID, colName := e.resolveCollection(ctx, p.ID, stream, b)
				for _, locale := range evalLocales(opts, b, p) {
					text := b.Text(locale)
					if isBlank(text) {
						continue
					}
					if err := visit(p, stream, b, locale, text, colID, colName); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// resolveStreams returns the streams to walk for a project: "main" always, plus
// each pilot stream named in opts that the project actually has.
func (e *Engine) resolveStreams(ctx context.Context, projectID string, opts EvalOptions) []string {
	streams := []string{"main"}
	pilots := opts.PilotStreams[projectID]
	if len(pilots) == 0 {
		return streams
	}
	existing := map[string]bool{"main": true}
	if list, err := e.blocks.ListStreams(ctx, projectID, false); err == nil {
		for _, s := range list {
			existing[s.Name] = true
		}
	}
	seen := map[string]bool{"main": true}
	for _, name := range pilots {
		if name != "" && existing[name] && !seen[name] {
			seen[name] = true
			streams = append(streams, name)
		}
	}
	return streams
}

// resolveCollection maps a block to its collection via the BlockSource when it
// implements CollectionResolver; otherwise (or on lookup failure) the block
// groups under its item name with an empty collection ID.
func (e *Engine) resolveCollection(ctx context.Context, projectID, stream string, b *store.StoredBlock) (id, name string) {
	cr, ok := e.blocks.(CollectionResolver)
	if ok && b.ItemName != "" {
		if item, err := cr.GetItem(ctx, projectID, stream, b.ItemName); err == nil && item != nil && item.CollectionID != "" {
			if col, err := cr.GetCollection(ctx, projectID, item.CollectionID); err == nil && col != nil {
				return col.ID, col.Name
			}
			return item.CollectionID, item.CollectionID
		}
	}
	return "", b.ItemName
}

// evalLocales returns the locales a block is evaluated in: opts.Locales when set,
// otherwise the block's own source locale (falling back to the project's default
// source language).
func evalLocales(opts EvalOptions, b *store.StoredBlock, p *store.Project) []model.LocaleID {
	if len(opts.Locales) > 0 {
		return opts.Locales
	}
	if b.SourceLocale != "" {
		return []model.LocaleID{b.SourceLocale}
	}
	if p != nil && p.DefaultSourceLanguage != "" {
		return []model.LocaleID{p.DefaultSourceLanguage}
	}
	return nil
}
