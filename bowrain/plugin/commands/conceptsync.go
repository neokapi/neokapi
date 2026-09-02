package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/ref/refcache"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	bproject "github.com/neokapi/neokapi/host/venue/project"
	bconn "github.com/neokapi/neokapi/host/venue/source"
	"github.com/neokapi/neokapi/terms"
	"golang.org/x/sync/errgroup"
)

// conceptsync.go folds the workspace knowledge graph into the ordinary project
// sync. A pull (kapi pull, kapi sync's pull phase) fetches the workspace's
// governed concepts + relations and writes them into the project's bound
// terms, recording a baseline in the sync cache so a later push can diff
// against it. A push (kapi push, kapi sync's push phase) reconciles local
// terms edits against that baseline: ordinary edits (definitions, notes, new
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
	Concept terms.Concept `json:"concept"`
}

type conceptDeletePayload struct {
	ConceptID string `json:"concept_id"`
}

type termAddPayload struct {
	ConceptID string     `json:"concept_id"`
	Term      terms.Term `json:"term"`
}

type termRemovePayload struct {
	ConceptID string `json:"concept_id"`
	Locale    string `json:"locale"`
	Text      string `json:"text"`
}

type relationAddPayload struct {
	Relation terms.ConceptRelation `json:"relation"`
}

// ---------------------------------------------------------------------------
// Results
// ---------------------------------------------------------------------------

// PullConceptsResult holds the counts a concept pull reports, plus what the
// return leg into the committed terms source amounted to.
type PullConceptsResult struct {
	Concepts  int
	Terms     int
	Relations int
	// Projection reports the merge of the workspace's reviewed decisions into
	// the project's committed terms source. Zero-valued when the project has no
	// recipe path to project into, and Written is false whenever the merge
	// produced the bytes already in git.
	Projection cli.TermsProjectionResult
	// TermsRef is the terms component of the ref the server published for this
	// workspace at the moment of the pull. It is what a later governed push
	// asserts, and what replaces asking a timestamp whether a snapshot is
	// still current. Empty when the server did not publish one.
	TermsRef string
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
	// ChangesetUnchanged: the proposed ops were identical to the change-set
	// already in review, so ChangesetID names that one and nothing new was
	// created.
	ChangesetUnchanged bool
	DryRun             bool
}

// conceptPushOrigin is the origin every kapi-push concept change-set carries;
// the server admits one open change-set per origin.
const conceptPushOrigin = "kapi-push/concepts"

// changed reports whether the push had any concept-related work — used by the
// caller to decide whether to surface a concept summary at all.
func (r *PushConceptsResult) changed() bool {
	return r.ConceptsApplied > 0 || r.RelationsApplied > 0 || r.ConceptsProposed > 0
}

// ---------------------------------------------------------------------------
// Pull
// ---------------------------------------------------------------------------

// PullConcepts paginates the workspace concept search, fetches the typed
// relations touching the pulled concepts, writes them into tb (refreshing by
// concept ID), projects the reviewed decisions among them into the project's
// committed terms source, and returns the counts plus a baseline snapshot the
// caller records in the sync cache so a later push can diff against it. When
// dryRun is set it fetches and counts but writes nothing.
//
// The projection is the return leg's other half. Without it a pull writes the
// workspace's vocabulary into the gitignored store and stops there, so a
// decision a reviewer approved on the server can never reach the repository
// that asked for it. recipePath names the project to project into; empty skips
// the projection (a caller with no recipe in hand).
//
// tb is the project's terms store, handed in rather than opened: it is a schema
// of the project's one store, and the handle belongs to the caller's App.
func PullConcepts(ctx context.Context, client *apiclient.BowrainClient, tb *terms.SQLiteStore, recipePath string, dryRun bool) (*PullConceptsResult, *bproject.ConceptBaseline, error) {
	concepts, kept, err := fetchServerConcepts(ctx, client)
	if err != nil {
		return nil, nil, err
	}

	res := &PullConceptsResult{Relations: len(kept)}
	for _, c := range concepts {
		res.Concepts++
		res.Terms += len(c.Terms)
	}

	if !dryRun {
		if err := writeConceptsToTerms(ctx, tb, concepts, kept); err != nil {
			return nil, nil, err
		}
		if recipePath != "" && app != nil {
			proj, perr := app.ProjectReviewedConcepts(ctx, recipePath, concepts, kept)
			if perr != nil {
				return nil, nil, fmt.Errorf("project reviewed terminology: %w", perr)
			}
			res.Projection = proj
		}
		// The terms component the workspace stands at, taken from the server
		// rather than folded from what was just fetched. The two ought to
		// agree, and asking removes the "ought": a client-side fold that
		// disagreed by one relation would refuse every later governed push
		// with a conflict nobody could find. Best-effort — a server too old to
		// publish a ref leaves the component unobserved, and an unobserved
		// component asserts nothing.
		if current, rerr := client.Ref(ctx); rerr == nil {
			res.TermsRef = current.Terms
		}
	}

	return res, buildBaseline(concepts, kept), nil
}

// fetchServerConcepts reads the workspace's governed terminology without
// touching the local store: every concept, plus the typed relations whose
// endpoints were pulled (the terms store rejects dangling edges).
//
// Split out of PullConcepts so a PUSH can ask the same question. Terminology is
// recipe-owned — the local store is the authority — so the only thing a push
// needs from the server is what it already holds, to avoid proposing something
// twice. That is a re-askable fact, not state the client must have persisted.
func fetchServerConcepts(ctx context.Context, client *apiclient.BowrainClient) ([]terms.Concept, []terms.ConceptRelation, error) {
	known := map[string]bool{}
	var (
		concepts   []terms.Concept
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
	// Keep only edges whose endpoints were pulled (the terms store rejects dangling
	// relations).
	kept := make([]terms.ConceptRelation, 0, len(relations))
	for _, rel := range relations {
		if known[rel.SourceID] && known[rel.TargetID] {
			kept = append(kept, rel)
		}
	}
	return concepts, kept, nil
}

// writeConceptsToTerms writes every concept then every relation into the
// project's terms store, refreshing any concept already present. Relations are
// added after all concepts so the store's referential check (both endpoints must
// exist) is satisfied.
//
// It neither opens nor closes: the store is a schema of the project's one store
// and the App owns the pool, so closing it here would take the content memory,
// the block cache and the working set with it.
func writeConceptsToTerms(ctx context.Context, tb *terms.SQLiteStore, concepts []terms.Concept, relations []terms.ConceptRelation) error {
	if tb == nil {
		return errors.New("write concepts: the project has no terms store")
	}
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
func fetchConceptRelations(ctx context.Context, client *apiclient.BowrainClient, conceptIDs []string) ([]terms.ConceptRelation, error) {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(conceptPullRelationConcurrency)

	var (
		mu        sync.Mutex
		seen      = map[string]bool{}
		relations []terms.ConceptRelation
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
func buildBaseline(concepts []terms.Concept, relations []terms.ConceptRelation) *bproject.ConceptBaseline {
	b := &bproject.ConceptBaseline{
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

// conceptInfoToConcept maps a server concept DTO into the framework terms
// concept type, casting the term status/locale strings and parsing the RFC3339
// timestamps.
func conceptInfoToConcept(ci apiclient.ConceptInfo) terms.Concept {
	concept := terms.Concept{
		ID:         ci.ID,
		ProjectID:  ci.ProjectID,
		Domain:     ci.Domain,
		Definition: ci.Definition,
		Properties: ci.Properties,
	}
	for _, t := range ci.Terms {
		concept.Terms = append(concept.Terms, terms.Term{
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

// PushConcepts diffs the project's terms against baseline and pushes the
// changes: ordinary edits go up directly through the concept endpoints, governed
// edits are bundled into one submitted change-set. It returns nil when the
// project has no terms. When dryRun is set it reports the plan without writing
// or proposing anything.
//
// tb is the project's terms store, handed in rather than opened. A nil store is
// the one condition that legitimately skips — it is what the caller's HasTerms
// says when the project's vocabulary is empty, which used to be a stat of
// `.kapi/terms.db`. The file question stopped distinguishing anything: every
// subsystem's schema exists from the store's first open, so the store file being
// there says only that some command ran in this project.
//
// expectedTerms is the terms component of the ref this project last observed —
// the compare-and-swap assertion carried on the change-set submit. The plan
// below is a DIFF against the baseline, so a workspace whose terminology moved
// since makes every governed op a proposal about a state that no longer exists.
func PushConcepts(ctx context.Context, client *apiclient.BowrainClient, tb *terms.SQLiteStore, baseline *bproject.ConceptBaseline, expectedTerms string, dryRun bool) (*PushConceptsResult, error) {
	if tb == nil {
		return nil, nil
	}

	// An absent baseline used to mean "skip", which is why terminology could
	// never reach a workspace that had not first been pulled from: a fresh
	// workspace has nothing to pull, so the pull established no baseline, so
	// the push skipped — permanently.
	//
	// Terminology is RECIPE-OWNED: the local store is the authority. So the
	// baseline is not a precondition for acting, it is only what stops a push
	// proposing something the server already has. That is a re-askable fact,
	// so ask for it. A workspace holding nothing yields an empty baseline, and
	// every local concept is correctly a create — which is how seeding works.
	if baseline == nil {
		serverConcepts, serverRels, err := fetchServerConcepts(ctx, client)
		if err != nil {
			return nil, fmt.Errorf("read workspace terminology to seed against: %w", err)
		}
		baseline = buildBaseline(serverConcepts, serverRels)
	}

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

	name := "kapi push " + time.Now().UTC().Format("2006-01-02 15:04")
	desc := fmt.Sprintf("Governed terminology edits proposed by kapi push: %d operation(s).", len(plan.governed))
	return plan.apply(ctx, client, name, desc, expectedTerms)
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
func (p pushPlan) apply(ctx context.Context, client *apiclient.BowrainClient, name, desc, expectedTerms string) (*PushConceptsResult, error) {
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
		// The origin admits one open change-set on the server: a re-push with
		// identical ops is idempotent (submit returns the open change-set),
		// and changed ops supersede it — drafts never pile up per push.
		cs, err := client.CreateChangeset(ctx, name, desc, conceptPushOrigin)
		if err != nil {
			return res, fmt.Errorf("create change-set: %w", err)
		}
		for _, op := range p.governed {
			if _, err := client.AppendChangesetOp(ctx, cs.ID, op); err != nil {
				return res, fmt.Errorf("append change-set op %q: %w", op.Op, err)
			}
		}
		submitted, err := client.SubmitChangeset(ctx, cs.ID, expectedTerms)
		if err != nil {
			return res, fmt.Errorf("submit change-set %s: %w", cs.ID, err)
		}
		res.ConceptsProposed = len(p.governed)
		res.ChangesetID = submitted.ID
		res.ChangesetUnchanged = submitted.ID != cs.ID
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
func buildPushPlan(local []terms.Concept, localRels []terms.ConceptRelation, baseline *bproject.ConceptBaseline) pushPlan {
	var plan pushPlan

	localByID := make(map[string]terms.Concept, len(local))
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
func diffConceptTerms(conceptID string, local terms.Concept, base bproject.BaselineConcept) ([]apiclient.ChangeSetOpInput, []terms.Term) {
	baseByID := make(map[string]bproject.BaselineTerm, len(base.Terms))
	for _, bt := range base.Terms {
		baseByID[bt.TermIdentity()] = bt
	}
	localByID := make(map[string]terms.Term, len(local.Terms))
	for _, lt := range local.Terms {
		localByID[termIdentity(lt)] = lt
	}

	var govOps []apiclient.ChangeSetOpInput
	ordinary := make([]terms.Term, 0, len(local.Terms))

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
		if string(lt.Status) != bt.Status && terms.IsGovernedTransition(model.TermStatus(bt.Status), lt.Status) {
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
func diffRelations(local []terms.ConceptRelation, baseline *bproject.ConceptBaseline) ([]relationAdd, []relationRemove, []apiclient.ChangeSetOpInput) {
	localByID := make(map[string]terms.ConceptRelation, len(local))
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
// claimed into a workspace or has no auth), pulls into the bound terms, and
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
	client, err := knowledgeClient(proj)
	if err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, nil, nil
	}
	tb, err := projectTerms(ctx, proj)
	if err != nil {
		return nil, nil, err
	}
	res, baseline, err := PullConcepts(ctx, client, tb, proj.Layout.RecipePath, dryRun)
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
// and diffs the bound terms against the sync-cache baseline. It skips when no
// baseline has been pulled yet. On a real push it fills in the change-set review
// URL.
// PushProjectConcepts reconciles the project's terminology into its workspace.
//
// Exported because a push arrives by two routes and terminology must travel on
// both. `kapi-bowrain push` runs the cobra command, which calls this after the
// content transport; `kapi push` is dispatched over the Mode-C daemon RPC,
// which called it nowhere — so terminology never reached a workspace through
// the verb everyone actually uses. Same shape as the context content type
// before #1625.
func PushProjectConcepts(ctx context.Context, proj *bproject.Project, dryRun bool) (*PushConceptsResult, error) {
	return conceptPush(ctx, proj, dryRun)
}

// PullProjectConcepts snapshots the workspace's terminology into the project.
//
// Exported for the same reason as its push counterpart, and it was missing for
// longer: a pull arrives by two routes, and only `kapi-bowrain pull` folded the
// terminology in. `kapi pull` — the verb everyone runs — took no snapshot, so
// no baseline was ever recorded from it, and every downstream reader of that
// baseline (the offline terminology gate, `kapi status`'s terms line, the
// push's diff) behaved as though the workspace had never been pulled from.
//
// The caller must hand the returned baseline to the connector via
// SetConceptBaseline so the connector's single Close() persists it.
func PullProjectConcepts(ctx context.Context, proj *bproject.Project, dryRun bool) (*PullConceptsResult, *bproject.ConceptBaseline, error) {
	return conceptPull(ctx, proj, dryRun)
}

func conceptPush(ctx context.Context, proj *bproject.Project, dryRun bool) (*PushConceptsResult, error) {
	client, err := knowledgeClient(proj)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, nil
	}
	// No guard on the baseline. It used to return here, which is what kept
	// terminology out of a workspace that had never been pulled from: a fresh
	// workspace has nothing to pull, so no baseline was written, so the push
	// returned — permanently. PushConcepts asks the server for the baseline
	// when the cache has none, so an absent one now means "seed", not "skip".
	cache := bproject.LoadSyncCache(proj.Layout)
	tb, err := projectTermsIfAny(ctx, proj)
	if err != nil {
		return nil, err
	}
	res, err := PushConcepts(ctx, client, tb, cache.ConceptBaseline, observedTermsRef(proj), dryRun)
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

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// knowledgeClient builds the workspace knowledge client for a terminology
// fold, returning (nil, nil) for the conditions under which there is genuinely
// nothing to sync with, and an error for every condition under which there is
// something and we could not reach it.
//
// The two legitimate skips:
//
//   - The project is not claimed into a workspace (ErrNotWorkspaceClaimed). There is no
//     workspace graph; the recipe is the whole of its terminology.
//   - The project syncs by claim token. The graph is workspace-scoped and a
//     claim token addresses one project, so a CI job pushing with a claim token
//     structurally cannot read it — this is documented in NewKnowledgeClient,
//     not a misconfiguration to shout about on every push.
//
// Everything else — an absent credential, a token minted for another server —
// is a failure and must be returned as one. Swallowed with `return nil, nil`,
// it lets a push carry every block and none of the terminology while reporting
// success.
func knowledgeClient(proj *bproject.Project) (*apiclient.BowrainClient, error) {
	if bproject.LoadSyncCache(proj.Layout).ClaimToken != "" {
		return nil, nil
	}
	client, err := bconn.NewKnowledgeClient(proj)
	switch {
	case errors.Is(err, bconn.ErrNotWorkspaceClaimed):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("workspace terminology sync: %w", err)
	}
	return client, nil
}

// projectTerms returns the terms store a concept pull/push works against: the
// project's own, out of its one store.
//
// There is no path to resolve. Terminology lives in the project's store and
// every term-aware command reads it with no flag and no recipe entry — a
// profile's `terms:` is the one place a recipe names a file, and that binds a
// standalone vocabulary to a region of the context space, not the project's
// own.
//
// The handle belongs to the App: do not close it.
func projectTerms(ctx context.Context, proj *bproject.Project) (*terms.SQLiteStore, error) {
	if app == nil {
		return nil, errors.New("terminology sync has no host app, so the project store is unreachable")
	}
	db, err := app.ProjectDB(ctx, proj.Root)
	if err != nil {
		return nil, fmt.Errorf("open project store: %w", err)
	}
	return db.Terms(), nil
}

// projectTermsIfAny returns the project's terms store only when it holds
// something. A push over an empty vocabulary has nothing to propose, and this is
// the row question that replaced the stat of `.kapi/terms.db`.
func projectTermsIfAny(ctx context.Context, proj *bproject.Project) (*terms.SQLiteStore, error) {
	if app == nil {
		return nil, errors.New("terminology sync has no host app, so the project store is unreachable")
	}
	db, err := app.ProjectDB(ctx, proj.Root)
	if err != nil {
		return nil, fmt.Errorf("open project store: %w", err)
	}
	has, err := db.HasTerms(ctx)
	if err != nil || !has {
		return nil, err
	}
	return db.Terms(), nil
}

// observedTermsRef reads the terms component this project last observed, for
// the compare-and-swap on a governed push. Read from disk rather than from a
// connector because a terminology push can run with no content transport beside
// it; the value is only read here, so there is no writer to race.
func observedTermsRef(proj *bproject.Project) string {
	if proj.Recipe == nil || proj.Recipe.Server == nil {
		return ""
	}
	return refcache.Load(proj.Layout,
		config.NormalizeServerURL(proj.Recipe.Server.ServerURL()),
		proj.Recipe.Server.ProjectID()).Ref(bproject.ResolveStream("", proj.Recipe.Server.Stream)).Terms
}

// changesetURL builds a best-effort link to review a change-set in the web hub
// from the recipe's server + workspace.
//
// The path must be the web app's route, not the REST endpoint's shape: the
// only route the app serves is /<ws>/context/changes/<id>
// (contextChangeDetailRoute, "changes/$id" under the "context" parent), and the
// sub-nav calls the surface Changes. This is the one link the CLI hands a
// reviewer, so a spelling the app does not serve reads to them as a push that
// did nothing.
// An empty return is the contract every caller reads as "report the id alone".
// A partially-built URL would be worse than none: missing its host or its
// workspace segment it resolves to a page that does not exist, which reads to a
// reviewer as a proposal that went nowhere rather than as a hub the project is
// not connected to.
func changesetURL(proj *bproject.Project, changesetID string) string {
	if proj == nil || proj.Recipe == nil || changesetID == "" {
		return ""
	}
	server := proj.Recipe.Server.ServerURL()
	workspace := proj.Recipe.Server.Workspace()
	if server == "" || workspace == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/context/changes/%s", strings.TrimRight(server, "/"), workspace, changesetID)
}

// newOp marshals an op payload into the change-set op input. The payloads are
// plain structs with no unmarshalable fields, so the marshal cannot fail.
func newOp(op string, payload any) apiclient.ChangeSetOpInput {
	raw, _ := json.Marshal(payload)
	return apiclient.ChangeSetOpInput{Op: op, Payload: raw}
}

// termIdentity keys a term by locale + lowered text, matching the server's
// governed-transition identity for terms.
func termIdentity(t terms.Term) string {
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
func conceptHasGovernedTerm(c terms.Concept) bool {
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
func ordinaryConceptChanged(local terms.Concept, base bproject.BaselineConcept, ordinaryTerms []terms.Term) bool {
	if local.Domain != base.Domain || local.Definition != base.Definition {
		return true
	}
	return termsSignature(ordinaryTerms) != baselineTermsSignature(base.Terms)
}

// termsSignature is an order-independent canonical signature of a terms list,
// used to compare the would-be PUT against the baseline.
func termsSignature(terms []terms.Term) string {
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
func termsToInfo(terms []terms.Term) []apiclient.TermInfo {
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
func baselineTermToTerm(t bproject.BaselineTerm) terms.Term {
	return terms.Term{
		Text:         t.Text,
		Locale:       model.LocaleID(t.Locale),
		Status:       model.TermStatus(t.Status),
		PartOfSpeech: t.PartOfSpeech,
		Gender:       t.Gender,
		Note:         t.Note,
	}
}
