package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// conceptsync.go folds the workspace knowledge graph into the ordinary project
// sync. A pull (kapi pull, kapi sync's pull phase) fetches the workspace's
// governed concepts + relations and writes them into the project's bound
// termbase, recording a baseline in the sync cache so a later push can diff
// against it. A push (kapi push, kapi sync's push phase) reconciles local
// termbase edits against that baseline: ordinary edits (definitions, notes, new
// proposed terms, non-governed relations) go up directly through the concept
// REST endpoints, while governed edits (a term banned/promoted, a forbidden
// status removed, a REPLACED_BY relation, a concept delete) are bundled into a
// single submitted change-set so they travel the same reviewed path the web hub
// enforces.

const (
	// conceptPullPageSize is the page size used when paginating the workspace
	// concept search during a concept pull.
	conceptPullPageSize = 200

	// conceptPullRelationConcurrency bounds the number of concurrent relation
	// fetches issued while snapshotting the workspace graph, so a large
	// workspace is pulled in O(pages + concepts/concurrency) round-trips rather
	// than one serial request per concept.
	conceptPullRelationConcurrency = 8
)

// Change-set op type wire strings (mirror knowledge.OpType; the plugin module
// cannot import the server-side knowledge package, so the stable wire strings
// are declared here).
const (
	opConceptCreate = "concept.create"
	opConceptDelete = "concept.delete"
	opTermAdd       = "term.add"
	opTermRemove    = "term.remove"
	opTermStatus    = "term.status"
	opRelationAdd   = "relation.add"
)

// ---------------------------------------------------------------------------
// Change-set op payloads (mirror knowledge.*Payload JSON shapes)
// ---------------------------------------------------------------------------

type termStatusPayload struct {
	ConceptID string `json:"concept_id"`
	Locale    string `json:"locale"`
	Text      string `json:"text"`
	From      string `json:"from"`
	To        string `json:"to"`
}

type conceptCreatePayload struct {
	Concept termbase.Concept `json:"concept"`
}

type conceptDeletePayload struct {
	ConceptID string `json:"concept_id"`
}

type termAddPayload struct {
	ConceptID string        `json:"concept_id"`
	Term      termbase.Term `json:"term"`
}

type termRemovePayload struct {
	ConceptID string `json:"concept_id"`
	Locale    string `json:"locale"`
	Text      string `json:"text"`
}

type relationAddPayload struct {
	Relation termbase.ConceptRelation `json:"relation"`
}

// ---------------------------------------------------------------------------
// Results
// ---------------------------------------------------------------------------

// PullConceptsResult holds the counts a concept pull reports.
type PullConceptsResult struct {
	Concepts  int
	Terms     int
	Relations int
}

// PushConceptsResult holds what a concept push applied directly versus proposed
// through a change-set. ConceptsApplied/RelationsApplied are the ordinary edits
// written straight to the concept endpoints; ConceptsProposed is the number of
// governed ops bundled into ChangesetID (with ChangesetURL a best-effort link to
// review it). DryRun reports a dry-run plan rather than executed work.
type PushConceptsResult struct {
	ConceptsApplied  int
	RelationsApplied int
	ConceptsProposed int
	ChangesetID      string
	ChangesetURL     string
	DryRun           bool
}

// changed reports whether the push had any concept-related work — used by the
// caller to decide whether to surface a concept summary at all.
func (r *PushConceptsResult) changed() bool {
	return r.ConceptsApplied > 0 || r.RelationsApplied > 0 || r.ConceptsProposed > 0
}

// ---------------------------------------------------------------------------
// Pull
// ---------------------------------------------------------------------------

// PullConcepts paginates the workspace concept search, fetches the typed
// relations touching the pulled concepts, writes them into the SQLite termbase
// at tbPath (refreshing by concept ID), and returns the counts plus a baseline
// snapshot the caller records in the sync cache so a later push can diff against
// it. When dryRun is set it fetches and counts but writes nothing.
func PullConcepts(ctx context.Context, client *apiclient.BowrainClient, tbPath string, dryRun bool) (*PullConceptsResult, *bproject.ConceptBaseline, error) {
	known := map[string]bool{}
	var (
		concepts   []termbase.Concept
		conceptIDs []string
	)

	offset := 0
	for {
		page, err := client.ListConcepts(ctx, apiclient.ListConceptsParams{
			Offset: offset,
			Limit:  conceptPullPageSize,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("list workspace concepts: %w", err)
		}
		if len(page.Concepts) == 0 {
			break
		}
		for _, ci := range page.Concepts {
			c := conceptInfoToConcept(ci)
			concepts = append(concepts, c)
			known[c.ID] = true
			conceptIDs = append(conceptIDs, c.ID)
		}
		offset += len(page.Concepts)
		if offset >= page.TotalCount || len(page.Concepts) < conceptPullPageSize {
			break
		}
	}

	relations, err := fetchConceptRelations(ctx, client, conceptIDs)
	if err != nil {
		return nil, nil, err
	}
	// Keep only edges whose endpoints were pulled (the termbase rejects dangling
	// relations).
	kept := make([]termbase.ConceptRelation, 0, len(relations))
	for _, rel := range relations {
		if known[rel.SourceID] && known[rel.TargetID] {
			kept = append(kept, rel)
		}
	}

	res := &PullConceptsResult{Relations: len(kept)}
	for _, c := range concepts {
		res.Concepts++
		res.Terms += len(c.Terms)
	}

	if !dryRun {
		if err := writeConceptsToTermbase(ctx, tbPath, concepts, kept); err != nil {
			return nil, nil, err
		}
	}

	return res, buildBaseline(concepts, kept), nil
}

// writeConceptsToTermbase opens (creating the directory if needed) the SQLite
// termbase at tbPath and writes every concept then every relation, refreshing
// any concept already present. Relations are added after all concepts so the
// termbase's referential check (both endpoints must exist) is satisfied.
func writeConceptsToTermbase(ctx context.Context, tbPath string, concepts []termbase.Concept, relations []termbase.ConceptRelation) error {
	if err := os.MkdirAll(filepath.Dir(tbPath), 0o755); err != nil {
		return fmt.Errorf("create termbase directory: %w", err)
	}
	tb, err := termbase.NewSQLiteTermBase(tbPath)
	if err != nil {
		return fmt.Errorf("open termbase: %w", err)
	}
	defer tb.Close()

	for _, c := range concepts {
		if err := tb.AddConcept(ctx, c); err != nil {
			return fmt.Errorf("write concept %s: %w", c.ID, err)
		}
	}
	for _, rel := range relations {
		if err := tb.AddRelation(ctx, rel); err != nil {
			return fmt.Errorf("write relation %s: %w", rel.ID, err)
		}
	}
	return nil
}

// fetchConceptRelations fetches the typed relations touching every concept in
// conceptIDs using a bounded parallel fan-out, de-duplicated by relation ID and
// sorted by ID so the pull stays deterministic despite the concurrent fetch.
func fetchConceptRelations(ctx context.Context, client *apiclient.BowrainClient, conceptIDs []string) ([]termbase.ConceptRelation, error) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(conceptPullRelationConcurrency)

	var (
		mu        sync.Mutex
		seen      = map[string]bool{}
		relations []termbase.ConceptRelation
	)

	for _, id := range conceptIDs {
		g.Go(func() error {
			rels, err := client.ListConceptRelations(ctx, id, "", "")
			if err != nil {
				return fmt.Errorf("list relations for concept %s: %w", id, err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, rel := range rels {
				if rel.ID == "" || seen[rel.ID] {
					continue
				}
				seen[rel.ID] = true
				relations = append(relations, rel)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	sort.Slice(relations, func(i, j int) bool { return relations[i].ID < relations[j].ID })
	return relations, nil
}

// buildBaseline snapshots the pulled concepts + relations into the diff-relevant
// baseline recorded in the sync cache.
func buildBaseline(concepts []termbase.Concept, relations []termbase.ConceptRelation) *bproject.ConceptBaseline {
	b := &bproject.ConceptBaseline{
		PulledAt:  time.Now().UTC(),
		Concepts:  make(map[string]bproject.BaselineConcept, len(concepts)),
		Relations: make(map[string]bproject.BaselineRelation, len(relations)),
	}
	for _, c := range concepts {
		bc := bproject.BaselineConcept{Domain: c.Domain, Definition: c.Definition}
		for _, t := range c.Terms {
			bc.Terms = append(bc.Terms, bproject.BaselineTerm{
				Text:         t.Text,
				Locale:       string(t.Locale),
				Status:       string(t.Status),
				PartOfSpeech: t.PartOfSpeech,
				Gender:       t.Gender,
				Note:         t.Note,
			})
		}
		b.Concepts[c.ID] = bc
	}
	for _, r := range relations {
		b.Relations[r.ID] = bproject.BaselineRelation{
			SourceID:     r.SourceID,
			TargetID:     r.TargetID,
			RelationType: r.RelationType,
			Note:         r.Note,
		}
	}
	return b
}

// conceptInfoToConcept maps a server concept DTO into the framework termbase
// concept type, casting the term status/locale strings and parsing the RFC3339
// timestamps.
func conceptInfoToConcept(ci apiclient.ConceptInfo) termbase.Concept {
	concept := termbase.Concept{
		ID:         ci.ID,
		ProjectID:  ci.ProjectID,
		Domain:     ci.Domain,
		Definition: ci.Definition,
		Properties: ci.Properties,
	}
	for _, t := range ci.Terms {
		concept.Terms = append(concept.Terms, termbase.Term{
			Text:         t.Text,
			Locale:       model.LocaleID(t.Locale),
			Status:       model.TermStatus(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		})
	}
	if ts, err := time.Parse(time.RFC3339, ci.CreatedAt); err == nil {
		concept.CreatedAt = ts
	}
	if ts, err := time.Parse(time.RFC3339, ci.UpdatedAt); err == nil {
		concept.UpdatedAt = ts
	}
	return concept
}

// ---------------------------------------------------------------------------
// Push
// ---------------------------------------------------------------------------

// PushConcepts diffs the local termbase at tbPath against baseline and pushes
// the changes: ordinary edits go up directly through the concept endpoints,
// governed edits are bundled into one submitted change-set. It returns nil when
// there is no baseline (a pull must run first) or no local termbase. When dryRun
// is set it reports the plan without writing or proposing anything.
func PushConcepts(ctx context.Context, client *apiclient.BowrainClient, tbPath string, baseline *bproject.ConceptBaseline, dryRun bool) (*PushConceptsResult, error) {
	if baseline == nil {
		return nil, nil
	}
	if _, err := os.Stat(tbPath); err != nil {
		return nil, nil
	}

	tb, err := termbase.NewSQLiteTermBase(tbPath)
	if err != nil {
		return nil, fmt.Errorf("open termbase: %w", err)
	}
	defer tb.Close()

	local, err := tb.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("read local concepts: %w", err)
	}
	localRels, err := tb.ListRelations(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("read local relations: %w", err)
	}

	plan := buildPushPlan(local, localRels, baseline)
	if plan.isEmpty() {
		return &PushConceptsResult{DryRun: dryRun}, nil
	}
	if dryRun {
		return &PushConceptsResult{
			ConceptsApplied:  len(plan.creates) + len(plan.updates),
			RelationsApplied: len(plan.relAdds) + len(plan.relRemoves),
			ConceptsProposed: len(plan.governed),
			DryRun:           true,
		}, nil
	}

	name := fmt.Sprintf("kapi push %s", time.Now().UTC().Format("2006-01-02 15:04"))
	desc := fmt.Sprintf("Governed terminology edits proposed by kapi push: %d operation(s).", len(plan.governed))
	return plan.apply(ctx, client, name, desc)
}

// pushPlan is the classified diff a concept push executes: ordinary edits that
// go up directly, plus governed ops bundled into one change-set.
type pushPlan struct {
	creates    []apiclient.CreateConceptParams // ordinary new concepts
	updates    []conceptUpdate                 // ordinary edits to existing concepts
	relAdds    []relationAdd                   // ordinary relation adds
	relRemoves []relationRemove                // relation removes
	governed   []apiclient.ChangeSetOpInput    // governed ops → change-set
}

type conceptUpdate struct {
	conceptID string
	params    apiclient.UpdateConceptParams
}

type relationAdd struct {
	sourceID string
	params   apiclient.AddRelationParams
}

type relationRemove struct {
	sourceID   string
	relationID string
}

func (p pushPlan) isEmpty() bool {
	return len(p.creates) == 0 && len(p.updates) == 0 &&
		len(p.relAdds) == 0 && len(p.relRemoves) == 0 && len(p.governed) == 0
}

// apply executes the plan against the server: ordinary creates/updates and
// relation edits land directly, then any governed ops are drafted into one
// change-set (create → append-op → submit) named name. It stops at the first
// error, returning the partial result so the caller can report what landed.
func (p pushPlan) apply(ctx context.Context, client *apiclient.BowrainClient, name, desc string) (*PushConceptsResult, error) {
	res := &PushConceptsResult{}

	for _, c := range p.creates {
		if _, err := client.CreateConcept(ctx, c); err != nil {
			return res, fmt.Errorf("create concept: %w", err)
		}
		res.ConceptsApplied++
	}
	for _, u := range p.updates {
		if err := client.UpdateConcept(ctx, u.conceptID, u.params); err != nil {
			return res, fmt.Errorf("update concept %s: %w", u.conceptID, err)
		}
		res.ConceptsApplied++
	}
	for _, a := range p.relAdds {
		if _, err := client.AddRelation(ctx, a.sourceID, a.params); err != nil {
			return res, fmt.Errorf("add relation on %s: %w", a.sourceID, err)
		}
		res.RelationsApplied++
	}
	for _, r := range p.relRemoves {
		if err := client.RemoveRelation(ctx, r.sourceID, r.relationID); err != nil {
			return res, fmt.Errorf("remove relation %s: %w", r.relationID, err)
		}
		res.RelationsApplied++
	}

	if len(p.governed) > 0 {
		cs, err := client.CreateChangeset(ctx, name, desc)
		if err != nil {
			return res, fmt.Errorf("create change-set: %w", err)
		}
		for _, op := range p.governed {
			if _, err := client.AppendChangesetOp(ctx, cs.ID, op); err != nil {
				return res, fmt.Errorf("append change-set op %q: %w", op.Op, err)
			}
		}
		if _, err := client.SubmitChangeset(ctx, cs.ID); err != nil {
			return res, fmt.Errorf("submit change-set %s: %w", cs.ID, err)
		}
		res.ConceptsProposed = len(p.governed)
		res.ChangesetID = cs.ID
	}

	return res, nil
}

// buildPushPlan classifies every local concept/relation change against the
// baseline. The classification is precise and never silently clobbers a governed
// edit: a governed term transition, a governed term add/remove, a REPLACED_BY
// relation, a concept create carrying a governed term, and a concept delete all
// become change-set ops, while the ordinary remainder goes up directly. An
// ordinary concept update keeps any governed terms pinned to their baseline
// status so the direct PUT never entails a governed transition the server would
// refuse with a 409.
func buildPushPlan(local []termbase.Concept, localRels []termbase.ConceptRelation, baseline *bproject.ConceptBaseline) pushPlan {
	var plan pushPlan

	localByID := make(map[string]termbase.Concept, len(local))
	localIDs := make([]string, 0, len(local))
	for _, c := range local {
		localByID[c.ID] = c
		localIDs = append(localIDs, c.ID)
	}
	sort.Strings(localIDs)

	for _, id := range localIDs {
		c := localByID[id]
		base, inBaseline := baseline.Concepts[id]
		if !inBaseline {
			// A brand-new local concept. Creating a term already forbidden or
			// preferred is governed (the direct POST would 409), so route the
			// whole concept through a change-set; otherwise create it directly.
			if conceptHasGovernedTerm(c) {
				plan.governed = append(plan.governed, newOp(opConceptCreate, conceptCreatePayload{Concept: c}))
			} else {
				plan.creates = append(plan.creates, apiclient.CreateConceptParams{
					ProjectID:  c.ProjectID,
					Domain:     c.Domain,
					Definition: c.Definition,
					Terms:      termsToInfo(c.Terms),
				})
			}
			continue
		}

		govOps, ordinaryTerms := diffConceptTerms(id, c, base)
		plan.governed = append(plan.governed, govOps...)
		if ordinaryConceptChanged(c, base, ordinaryTerms) {
			plan.updates = append(plan.updates, conceptUpdate{
				conceptID: id,
				params: apiclient.UpdateConceptParams{
					Domain:     c.Domain,
					Definition: c.Definition,
					Terms:      termsToInfo(ordinaryTerms),
				},
			})
		}
	}

	// Concepts present in the baseline but gone locally → governed delete.
	for _, id := range baseline.SortedConceptIDs() {
		if _, ok := localByID[id]; !ok {
			plan.governed = append(plan.governed, newOp(opConceptDelete, conceptDeletePayload{ConceptID: id}))
		}
	}

	adds, removes, relGov := diffRelations(localRels, baseline)
	plan.relAdds = adds
	plan.relRemoves = removes
	plan.governed = append(plan.governed, relGov...)

	return plan
}

// diffConceptTerms classifies the term-level changes of an existing concept. It
// returns the governed ops and the terms list a (possibly emitted) ordinary
// update PUT should carry — a list constructed so the PUT never entails a
// governed transition: a term with a pending governed transition keeps its
// baseline status (ordinary metadata edits still applied), a governed term add
// is excluded (it rides a term.add op), and a governed term removal keeps the
// term (the change-set's term.remove op removes it).
func diffConceptTerms(conceptID string, local termbase.Concept, base bproject.BaselineConcept) ([]apiclient.ChangeSetOpInput, []termbase.Term) {
	baseByID := make(map[string]bproject.BaselineTerm, len(base.Terms))
	for _, bt := range base.Terms {
		baseByID[bt.TermIdentity()] = bt
	}
	localByID := make(map[string]termbase.Term, len(local.Terms))
	for _, lt := range local.Terms {
		localByID[termIdentity(lt)] = lt
	}

	var govOps []apiclient.ChangeSetOpInput
	ordinary := make([]termbase.Term, 0, len(local.Terms))

	for _, lt := range local.Terms {
		key := termIdentity(lt)
		bt, inBase := baseByID[key]
		if !inBase {
			if isGovernedStatus(lt.Status) {
				govOps = append(govOps, newOp(opTermAdd, termAddPayload{ConceptID: conceptID, Term: lt}))
				continue
			}
			ordinary = append(ordinary, lt)
			continue
		}
		if string(lt.Status) != bt.Status && termbase.IsGovernedTransition(model.TermStatus(bt.Status), lt.Status) {
			govOps = append(govOps, newOp(opTermStatus, termStatusPayload{
				ConceptID: conceptID,
				Locale:    string(lt.Locale),
				Text:      lt.Text,
				From:      bt.Status,
				To:        string(lt.Status),
			}))
			neutral := lt
			neutral.Status = model.TermStatus(bt.Status)
			ordinary = append(ordinary, neutral)
			continue
		}
		ordinary = append(ordinary, lt)
	}

	// Terms removed locally that carried a FORBIDDEN status: route the removal
	// through the change-set and keep the term in the ordinary PUT. Only
	// un-forbidding is governed server-side (governedConceptUpdate); removing a
	// preferred or any other term applies directly via the PUT below.
	for _, bt := range base.Terms {
		if _, ok := localByID[bt.TermIdentity()]; ok {
			continue
		}
		if model.TermStatus(bt.Status) == model.TermForbidden {
			govOps = append(govOps, newOp(opTermRemove, termRemovePayload{
				ConceptID: conceptID,
				Locale:    bt.Locale,
				Text:      bt.Text,
			}))
			ordinary = append(ordinary, baselineTermToTerm(bt))
		}
	}

	return govOps, ordinary
}

// diffRelations classifies relation changes. An added REPLACED_BY relation is
// governed; every other add is an ordinary direct add. Removals are ordinary
// (the server ungates relation deletes). Relations are matched by ID; in-place
// edits to an existing edge are not diffed (edges are effectively immutable).
func diffRelations(local []termbase.ConceptRelation, baseline *bproject.ConceptBaseline) ([]relationAdd, []relationRemove, []apiclient.ChangeSetOpInput) {
	localByID := make(map[string]termbase.ConceptRelation, len(local))
	localIDs := make([]string, 0, len(local))
	for _, r := range local {
		if r.ID == "" {
			continue
		}
		localByID[r.ID] = r
		localIDs = append(localIDs, r.ID)
	}
	sort.Strings(localIDs)

	var (
		adds    []relationAdd
		removes []relationRemove
		gov     []apiclient.ChangeSetOpInput
	)

	for _, id := range localIDs {
		if _, ok := baseline.Relations[id]; ok {
			continue
		}
		r := localByID[id]
		if r.RelationType == graph.LabelReplacedBy {
			gov = append(gov, newOp(opRelationAdd, relationAddPayload{Relation: r}))
			continue
		}
		adds = append(adds, relationAdd{
			sourceID: r.SourceID,
			params: apiclient.AddRelationParams{
				TargetID:     r.TargetID,
				RelationType: r.RelationType,
				Note:         r.Note,
				Validity:     r.Validity,
			},
		})
	}

	baseIDs := make([]string, 0, len(baseline.Relations))
	for id := range baseline.Relations {
		baseIDs = append(baseIDs, id)
	}
	sort.Strings(baseIDs)
	for _, id := range baseIDs {
		if _, ok := localByID[id]; ok {
			continue
		}
		br := baseline.Relations[id]
		removes = append(removes, relationRemove{sourceID: br.SourceID, relationID: id})
	}

	return adds, removes, gov
}

// ---------------------------------------------------------------------------
// Project wrappers (skip-aware)
// ---------------------------------------------------------------------------

// conceptPull runs the project-level concept pull: it builds the workspace
// knowledge client (skipping silently — returning nil — when the project is not
// claimed into a workspace or has no auth), pulls into the bound termbase, and
// returns the baseline the caller must hand to the sync connector via
// SetConceptBaseline so the connector's single final Close() persists it
// alongside the block-sync state. A genuine fetch error is surfaced; a skip is
// not. The returned baseline is nil on a dry run (nothing was written, so a
// later push must not diff against it) and nil on a skip.
//
// conceptPull deliberately does NOT write the sync cache itself: the sync
// connector owns the in-memory cache and flushes it once on Close, so a separate
// load-mutate-save here would be clobbered by that deferred Close (the connector
// re-saves its own copy, which never carried the baseline). Returning the
// baseline keeps the connector the single writer of the cache.
func conceptPull(ctx context.Context, proj *bproject.Project, dryRun bool) (*PullConceptsResult, *bproject.ConceptBaseline, error) {
	client, err := bconn.NewKnowledgeClient(proj)
	if err != nil {
		return nil, nil, nil
	}
	tbPath, err := projectTermbasePath(proj)
	if err != nil {
		return nil, nil, err
	}
	res, baseline, err := PullConcepts(ctx, client, tbPath, dryRun)
	if err != nil {
		return nil, nil, err
	}
	if dryRun {
		return res, nil, nil
	}
	return res, baseline, nil
}

// conceptPush runs the project-level concept push: it builds the workspace
// knowledge client (skipping silently when the project is not workspace-claimed)
// and diffs the bound termbase against the sync-cache baseline. It skips when no
// baseline has been pulled yet. On a real push it fills in the change-set review
// URL.
func conceptPush(ctx context.Context, proj *bproject.Project, dryRun bool) (*PushConceptsResult, error) {
	client, err := bconn.NewKnowledgeClient(proj)
	if err != nil {
		return nil, nil
	}
	cache := bproject.LoadSyncCache(proj.Layout)
	if cache.ConceptBaseline == nil {
		return nil, nil
	}
	tbPath, err := projectTermbasePath(proj)
	if err != nil {
		return nil, err
	}
	res, err := PushConcepts(ctx, client, tbPath, cache.ConceptBaseline, dryRun)
	if err != nil {
		return nil, err
	}
	if res == nil || !res.changed() {
		return nil, nil
	}
	if res.ChangesetID != "" {
		res.ChangesetURL = changesetURL(proj, res.ChangesetID)
	}
	return res, nil
}

// printConceptPushSummary writes a concept push summary to the command's stdout
// for the kapi sync path (which prints progress directly rather than through the
// structured output layer).
func printConceptPushSummary(cmd *cobra.Command, res *PushConceptsResult) {
	w := cmd.OutOrStdout()
	if res.ConceptsApplied > 0 || res.RelationsApplied > 0 {
		fmt.Fprintf(w, "Applied %d concept edit(s) and %d relation edit(s) directly\n",
			res.ConceptsApplied, res.RelationsApplied)
	}
	if res.ConceptsProposed > 0 {
		fmt.Fprintf(w, "Proposed %d governed edit(s) in change-set %s\n", res.ConceptsProposed, res.ChangesetID)
		if res.ChangesetURL != "" {
			fmt.Fprintf(w, "Review it at %s\n", res.ChangesetURL)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// projectTermbasePath returns the SQLite termbase file a concept pull/push uses.
// It mirrors the CLI's project-bound resolution: defaults.termbase from the
// recipe (relative to the project root), else the conventional
// <root>/.kapi/termbase.db.
func projectTermbasePath(proj *bproject.Project) (string, error) {
	if bound := proj.Recipe.Defaults.Termbase; bound != "" {
		if filepath.IsAbs(bound) {
			return bound, nil
		}
		return filepath.Join(proj.Root, bound), nil
	}
	return filepath.Join(proj.StateDir(), "termbase.db"), nil
}

// changesetURL builds a best-effort link to review a change-set in the web hub
// from the recipe's server + workspace.
func changesetURL(proj *bproject.Project, changesetID string) string {
	server := proj.Recipe.Server.ServerURL()
	workspace := proj.Recipe.Server.Workspace()
	if server == "" || workspace == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/changesets/%s", strings.TrimRight(server, "/"), workspace, changesetID)
}

// newOp marshals an op payload into the change-set op input. The payloads are
// plain structs with no unmarshalable fields, so the marshal cannot fail.
func newOp(op string, payload any) apiclient.ChangeSetOpInput {
	raw, _ := json.Marshal(payload)
	return apiclient.ChangeSetOpInput{Op: op, Payload: raw}
}

// termIdentity keys a term by locale + lowered text, matching the server's
// governed-transition identity for terms.
func termIdentity(t termbase.Term) string {
	return string(t.Locale) + "|" + strings.ToLower(t.Text)
}

// isGovernedStatus reports whether a term status is one a governed transition
// targets (forbidden or preferred) — the statuses whose creation/addition the
// server refuses on the direct path.
func isGovernedStatus(s model.TermStatus) bool {
	return s == model.TermForbidden || s == model.TermPreferred
}

// conceptHasGovernedTerm reports whether any of a concept's terms carries a
// governed (forbidden/preferred) status.
func conceptHasGovernedTerm(c termbase.Concept) bool {
	for _, t := range c.Terms {
		if isGovernedStatus(t.Status) {
			return true
		}
	}
	return false
}

// ordinaryConceptChanged reports whether a concept's ordinary, directly-pushable
// state (domain, definition, and the neutralized terms list) differs from the
// baseline. Properties are not diffed — the direct concept PUT does not carry
// them.
func ordinaryConceptChanged(local termbase.Concept, base bproject.BaselineConcept, ordinaryTerms []termbase.Term) bool {
	if local.Domain != base.Domain || local.Definition != base.Definition {
		return true
	}
	return termsSignature(ordinaryTerms) != baselineTermsSignature(base.Terms)
}

// termsSignature is an order-independent canonical signature of a terms list,
// used to compare the would-be PUT against the baseline.
func termsSignature(terms []termbase.Term) string {
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		parts = append(parts, strings.Join([]string{
			string(t.Locale), strings.ToLower(t.Text), string(t.Status), t.Note, t.PartOfSpeech, t.Gender,
		}, "\x1f"))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x1e")
}

func baselineTermsSignature(terms []bproject.BaselineTerm) string {
	parts := make([]string, 0, len(terms))
	for _, t := range terms {
		parts = append(parts, strings.Join([]string{
			t.Locale, strings.ToLower(t.Text), t.Status, t.Note, t.PartOfSpeech, t.Gender,
		}, "\x1f"))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\x1e")
}

// termsToInfo maps framework terms to the client's term DTO.
func termsToInfo(terms []termbase.Term) []apiclient.TermInfo {
	out := make([]apiclient.TermInfo, 0, len(terms))
	for _, t := range terms {
		out = append(out, apiclient.TermInfo{
			Text:         t.Text,
			Locale:       string(t.Locale),
			Status:       string(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		})
	}
	return out
}

// baselineTermToTerm reconstructs a framework term from its baseline snapshot.
func baselineTermToTerm(t bproject.BaselineTerm) termbase.Term {
	return termbase.Term{
		Text:         t.Text,
		Locale:       model.LocaleID(t.Locale),
		Status:       model.TermStatus(t.Status),
		PartOfSpeech: t.PartOfSpeech,
		Gender:       t.Gender,
		Note:         t.Note,
	}
}
