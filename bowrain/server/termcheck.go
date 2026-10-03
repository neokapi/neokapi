package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
)

// This file holds the ONE terms-aware term-compliance predicate the ship /
// compliant surfaces and the RV-E review re-check oracle share, so they can never
// disagree on whether a target respects the project's terminology governance.
//
// The predicate combines the two complementary directions the review loop already
// checks (review_recheck.go), lifted here so both surfaces import them:
//
//   - PRESENCE: the target CONTAINS a forbidden or competitor term — drawn from
//     the terms store (LookupAll, the same matcher the concept blast radius and RV-E
//     use) AND from the voice profile's own rules (core/profile.Findings: the
//     word rules its file carries and its patterns).
//   - ABSENCE: the source uses a concept that MANDATES a preferred or approved
//     rendering for the target locale and the target OMITS it, or the concept is
//     marked do-not-translate and the target does not keep the term verbatim.
//     Both are decided by coretools.TermCheckViolations, the function the
//     term-check tool itself decides with, so a translation the CLI gate passes
//     is compliant here and one it fails is a violation.
//
// The checks are deterministic and offline — no DB or LLM call is made per block.
// The terms store is snapshotted in-memory once per (workspace) and the voice profile
// resolved once per (workspace, locale) by the caller (see termGate), then reused
// across every block of the ship/compliant pass.

// blockTermCompliance is the terminology verdict for a block's committed target
// for tgtLoc. The target is a violation when it USES a forbidden or competitor
// term (presence) or OMITS every rendering its source concept accepts
// (absence, decided as term-check decides it), and compliant when it was
// checked and does neither. It is the single per-block predicate the dashboard ship-state pass,
// the compliant-rate derivation, the review queue, the bulk approve-passing
// endpoint, and the RV-E concept re-check oracle all call.
//
// tb is the terms governing tgtLoc, an in-memory snapshot resolved once by the
// caller (nil when no concept answers for the locale, and the terms half is
// skipped). profile is the voice profile resolved for tgtLoc once by the caller
// (nil when none is bound, and the vocabulary half is skipped). With neither,
// terminology does not govern the locale and the verdict claims nothing. A
// target in a governed locale with no text has nothing to check, so it is
// unchecked. A caller treats both as states of their own, neither compliant
// nor a violation.
func blockTermCompliance(ctx context.Context, block *model.Block, srcLoc, tgtLoc model.LocaleID, tb terms.Terminology, profile *coreprofile.VoiceProfile) store.TermCompliance {
	return blockTermComplianceWith(ctx, block, srcLoc, tgtLoc, tb, profile, nil)
}

// blockTermComplianceWith is blockTermCompliance with the term rules the
// project's recipe declares for tgtLoc (declared), which govern the locale
// beside the terms and the voice profile. The absence half holds the target to
// the rules the concepts give and the declared ones together, a declared rule
// standing in for a concept's rule on the same term (project.
// WithDeclaredTermRules), which is how kapi's terms gate resolves them. A
// declared rule marked advisory reports in kapi and never makes a violation
// here.
func blockTermComplianceWith(ctx context.Context, block *model.Block, srcLoc, tgtLoc model.LocaleID, tb terms.Terminology, profile *coreprofile.VoiceProfile, declared []coreprofile.TermRule) store.TermCompliance {
	if tb == nil && coreprofile.BlockRuleCount(profile) == 0 && len(declared) == 0 {
		return store.TermComplianceNotGoverned
	}
	if block == nil {
		return store.TermComplianceUnchecked
	}
	targetText := block.TargetText(tgtLoc)
	if strings.TrimSpace(targetText) == "" {
		return store.TermComplianceUnchecked
	}
	// PRESENCE (terms): a forbidden/competitor term appears in the target.
	if tb != nil && targetHasForbiddenTerm(ctx, tb, targetText, tgtLoc) {
		return store.TermComplianceViolation
	}
	// PRESENCE (voice profile): a forbidden/competitor rule or a prohibited style
	// pattern matches the target. core/profile.Findings is the canonical
	// deterministic gate.
	if profile != nil && len(coreprofile.Findings(profile, targetText, nil)) > 0 {
		return store.TermComplianceViolation
	}
	// ABSENCE (terms and the recipe's rules): the source uses a concept or a
	// declared rule whose mandated rendering for the target locale, or whose
	// do-not-translate term, is missing from the target.
	if targetMissingMandatedTerm(ctx, tb, declared, block.SourceText(), targetText, srcLoc, tgtLoc) {
		return store.TermComplianceViolation
	}
	return store.TermComplianceCompliant
}

// targetHasForbiddenTerm reports whether targetText (a translation in locale loc)
// contains any forbidden or competitor term drawn from tb. It runs the canonical
// terms LookupAll keyed on the target locale so a per-locale terminology
// decision is matched in the language it governs — the same oracle the concept
// blast radius and RV-E's presence direction use.
func targetHasForbiddenTerm(ctx context.Context, tb terms.Terminology, targetText string, loc model.LocaleID) bool {
	matches, err := tb.LookupAll(ctx, targetText, terms.LookupOptions{SourceLocale: loc})
	if err != nil {
		return false
	}
	for _, m := range matches {
		if m.Term.Status == model.TermForbidden || m.Term.CompetitorTerm {
			return true
		}
	}
	return false
}

// targetMissingMandatedTerm reports whether the target VIOLATES terminology by
// ABSENCE: the source uses a concept's term, and the target holds none of the
// renderings that concept accepts for tgtLoc. It decides exactly what the
// term-check tool decides, by deriving the rules with terms.RulesFromConcepts
// and asking coretools.TermCheckViolations, so a translation the CLI gate
// passes is compliant here and one it fails is a violation. That includes an
// admitted or approved term, a declared form of any rendering, an English
// source term's regular inflections, placeholder names read as syntax, and a
// do-not-translate term, which the target must keep verbatim. Redirection
// through USE_INSTEAD / REPLACED_BY relations is not followed, as in term-check.
//
// declared are the term rules the recipe declares for tgtLoc. They join the
// concepts' rules as kapi's gate joins them, and tb may be nil when they are
// all that governs the locale.
func targetMissingMandatedTerm(ctx context.Context, tb terms.Terminology, declared []coreprofile.TermRule, sourceText, targetText string, srcLoc, tgtLoc model.LocaleID) bool {
	if strings.TrimSpace(sourceText) == "" || strings.TrimSpace(targetText) == "" {
		return false
	}
	var rules []coreprofile.TermRule
	if tb != nil {
		if concepts, err := tb.Concepts(ctx); err == nil && len(concepts) > 0 {
			rules = terms.RulesFromConcepts(concepts, srcLoc, tgtLoc)
		}
	}
	rules = coreprofile.WithDeclaredTermRules(rules, declared)
	if len(rules) == 0 {
		return false
	}
	errs, _ := coretools.TermCheckViolations(&coretools.TermCheckConfig{
		TermRules:    rules,
		SourceLocale: srcLoc,
		TargetLocale: tgtLoc,
	}, sourceText, targetText)
	return len(errs) > 0
}

// termGate carries the governance context for one ship/compliant pass: an
// in-memory terms snapshot (resolved once), which target locales that snapshot
// governs (decided at most once per locale), and a per-locale voice profile
// resolver (resolved at most once per locale, then cached). It is the
// caller-side wrapper that keeps the shared blockTermCompliance predicate
// bounded: the pass resolves stores once, the predicate runs offline per block.
// A nil gate holds no governance, so every dimension it is asked about is not
// governed.
type termGate struct {
	srcLoc     model.LocaleID
	tb         terms.Terminology // in-memory snapshot; nil = the workspace holds no terms
	termsFP    string            // digest of the snapshot, for the gate fingerprint
	resolve    func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile
	profiles   map[model.LocaleID]*coreprofile.VoiceProfile
	profileSet map[model.LocaleID]bool
	// concepts is what tb indexes, read once, and governs caches per target
	// locale whether any of them answers for it.
	concepts     []terms.Concept
	conceptsRead bool
	governs      map[model.LocaleID]bool
	// declared is the term rules the project's recipe declares, as the last
	// push that applied them left them (store.RecipeTermRulesOf), and
	// declaredFP their encoded form, for the gate fingerprint.
	declared   coreprofile.RecipeTermRules
	declaredFP string
}

// withDeclared sets the recipe's term rules on the gate, which then governs
// every locale they answer for. A nil gate is returned as it is.
func (g *termGate) withDeclared(declared coreprofile.RecipeTermRules) *termGate {
	if g == nil || declared.Empty() {
		return g
	}
	g.declared = declared
	g.declaredFP, _ = declared.Encode()
	return g
}

// declaredFor returns the recipe's term rules for one target locale.
func (g *termGate) declaredFor(loc model.LocaleID) []coreprofile.TermRule {
	if g == nil {
		return nil
	}
	return g.declared.For(string(loc))
}

// newTermGate builds a gate around an already-resolved terms snapshot and a
// per-locale profile resolver. Either input may be absent (nil tb / nil
// resolve). termsFP digests the snapshot so the gate can name the governance a
// stored verdict was computed under.
func newTermGate(srcLoc model.LocaleID, tb terms.Terminology, termsFP string, resolve func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile) *termGate {
	return &termGate{
		srcLoc:     srcLoc,
		tb:         tb,
		termsFP:    termsFP,
		resolve:    resolve,
		profiles:   map[model.LocaleID]*coreprofile.VoiceProfile{},
		profileSet: map[model.LocaleID]bool{},
		governs:    map[model.LocaleID]bool{},
	}
}

// shipGateAlgorithm names the revision of the per-block ship-gate predicate.
// It rides in every gate fingerprint, so changing what blockFailsChecks or
// blockTermCompliance decide retires every stored verdict rather than leaving
// counters that were computed by code no longer running. Bump it whenever the
// predicate's answer can change for unchanged content.
const shipGateAlgorithm = "4"

// fingerprint names the governance in force for a set of target locales: the
// predicate's own revision, the source language it reads, the terms snapshot,
// and each rated locale's resolved voice profile. Two passes that agree on this
// string judge identical content identically, which is precisely the condition
// under which a stored verdict may be counted instead of recomputed.
//
// Locales are fingerprinted in the order given, so the caller sorts them; the
// alternative is a gate that changes with map iteration order and a corpus that
// is recomputed on every load.
func (g *termGate) fingerprint(ctx context.Context, locales []string) string {
	h := sha256.New()
	fmt.Fprintf(h, "algo=%s\n", shipGateAlgorithm)
	if g != nil {
		fmt.Fprintf(h, "src=%s\nterms=%s\n", g.srcLoc, g.termsFP)
		// Written only when the recipe declares rules, so a project without
		// them keeps the verdicts it has stored.
		if g.declaredFP != "" {
			fmt.Fprintf(h, "term_rules=%s\n", g.declaredFP)
		}
		for _, l := range locales {
			p := g.profileFor(ctx, model.LocaleID(l))
			if p == nil {
				fmt.Fprintf(h, "loc=%s profile=-\n", l)
				continue
			}
			fmt.Fprintf(h, "loc=%s profile=%s v=%d bar=%d at=%d\n",
				l, p.ID, p.Version, p.ComplianceBar(), p.UpdatedAt.UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// profileFor resolves (and caches) the voice profile for one target locale.
// The resolver runs at most once per locale across the whole pass — never per
// block — so the ship/compliant pass adds no per-block store reads.
func (g *termGate) profileFor(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile {
	if g == nil || g.resolve == nil {
		return nil
	}
	if g.profileSet[loc] {
		return g.profiles[loc]
	}
	p := g.resolve(ctx, loc)
	g.profiles[loc] = p
	g.profileSet[loc] = true
	return p
}

// compliance is block's terminology verdict for tgtLoc under the gate's
// governance, from the shared blockTermCompliance predicate. The terms snapshot
// takes part only for a locale it governs, the recipe's term rules for the
// locale always, and a nil gate governs nothing.
func (g *termGate) compliance(ctx context.Context, block *model.Block, tgtLoc model.LocaleID) store.TermCompliance {
	if g == nil {
		return store.TermComplianceNotGoverned
	}
	var tb terms.Terminology
	if g.snapshotGoverns(ctx, tgtLoc) {
		tb = g.tb
	}
	return blockTermComplianceWith(ctx, block, g.srcLoc, tgtLoc, tb, g.profileFor(ctx, tgtLoc), g.declaredFor(tgtLoc))
}

// snapshotGoverns reports whether the terms snapshot governs tgtLoc: whether at
// least one of its concepts answers for a translation from the project's source
// language into it (terms.RuleForConcept), the same derivation kapi's term rule
// resolution applies. A workspace holding terms for other languages only leaves
// tgtLoc ungoverned by terms.
func (g *termGate) snapshotGoverns(ctx context.Context, tgtLoc model.LocaleID) bool {
	if g == nil || g.tb == nil {
		return false
	}
	if governs, ok := g.governs[tgtLoc]; ok {
		return governs
	}
	if !g.conceptsRead {
		concepts, err := g.tb.Concepts(ctx)
		if err != nil {
			// The snapshot is in memory, so a read does not fail in practice.
			// Should one, the gate decides on no concepts and says so.
			slog.ErrorContext(ctx, "termcheck: terms snapshot unreadable; its locales read as ungoverned by terms",
				"error", err)
		}
		g.concepts, g.conceptsRead = concepts, true
	}
	governs := false
	for i := range g.concepts {
		if _, ok := terms.RuleForConcept(g.concepts[i], g.srcLoc, tgtLoc); ok {
			governs = true
			break
		}
	}
	g.governs[tgtLoc] = governs
	return governs
}

// termsGoverned reports whether terminology governs tgtLoc: the terms snapshot
// governs it, the recipe declares term rules for it, or the voice profile
// resolved for it holds a rule that applies to a block. It agrees with compliance, which is not governed for every target of a
// locale this reports false for, and it drives the compliance basis and the
// not-governed count.
func (g *termGate) termsGoverned(ctx context.Context, tgtLoc model.LocaleID) bool {
	if g == nil {
		return false
	}
	return g.snapshotGoverns(ctx, tgtLoc) || len(g.declaredFor(tgtLoc)) > 0 ||
		coreprofile.BlockRuleCount(g.profileFor(ctx, tgtLoc)) > 0
}

// voiceGoverned reports whether a voice profile applies at tgtLoc, which is what
// puts a voice bar on its blocks.
func (g *termGate) voiceGoverned(ctx context.Context, tgtLoc model.LocaleID) bool {
	return g.profileFor(ctx, tgtLoc) != nil
}

// resolveTermGate builds the terminology-governance gate for a project's
// ship/compliant pass: an in-memory snapshot of the workspace terms (one read,
// reused across every block and locale), a per-locale voice profile resolver
// (resolved at most once per locale) and the term rules the project's recipe
// declares. The gate is deterministic and offline, with no per-block DB or LLM
// call. Returns nil when the project has no terms store, no voice store and no
// recipe term rules, and a nil gate governs nothing.
func (s *Server) resolveTermGate(ctx context.Context, proj *store.Project, stream, wsID string) *termGate {
	if proj == nil {
		return nil
	}

	// Terms snapshot: read every concept once and index them in memory, so the
	// per-block LookupAll calls never touch the database. An unavailable or empty
	// terms leaves the snapshot nil (the terms store half of the predicate skips).
	var snap terms.Terminology
	var snapFP string
	if tb, err := s.workspaceTermsByID(ctx, wsID); err == nil && tb != nil {
		snap, snapFP = s.termSnapshots.snapshot(ctx, wsID, tb)
	}

	// Voice profile resolver: the same hierarchical binding ladder the editor and
	// worker resolve through (voicescope.Resolve), scoped per target locale.
	var resolve func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile
	if s.VoiceStore != nil {
		var wd voicescope.WorkspaceDefault
		if s.AuthStore != nil {
			wd = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
		}
		resolve = func(ctx context.Context, loc model.LocaleID) *coreprofile.VoiceProfile {
			profile, err := voicescope.Resolve(ctx, s.ContentStore, wd, s.VoiceStore, voicescope.Scope{
				WorkspaceID: wsID,
				ProjectID:   proj.ID,
				Stream:      stream,
				Locale:      loc,
			})
			if err != nil {
				return nil
			}
			return profile
		}
	}

	// The term rules the recipe declares, carried by the push that applied them.
	declared := store.RecipeTermRulesOf(proj)

	if snap == nil && resolve == nil && declared.Empty() {
		return nil
	}
	return newTermGate(proj.DefaultSourceLanguage, snap, snapFP, resolve).withDeclared(declared)
}

// snapshotTerms reads every concept from a workspace terms and indexes them
// in a fresh in-memory terms, so the ship/compliant pass can run LookupAll per
// block offline. Returns nil for an empty terms or a read failure (the caller
// then skips the terms store half of the predicate). Relations are not copied: the
// shared predicate does not follow USE_INSTEAD / REPLACED_BY redirection, matching
// the review loop's primitive check.
//
// The second return is a digest of exactly what was indexed — the concepts as
// the snapshot holds them, not as the store spells them — so a gate fingerprint
// changes if and only if the governance the predicate actually applies changed.
// A concept dropped by AddConcept is therefore absent from both the snapshot and
// the digest, which is the honest pairing: verdicts are counted as computed.
func snapshotTerms(ctx context.Context, tb terms.Store) (terms.Terminology, string) {
	snap, fp, _ := readTermSnapshot(ctx, tb)
	return snap, fp
}

// readTermSnapshot is snapshotTerms, reporting a read that failed.
func readTermSnapshot(ctx context.Context, tb terms.Store) (terms.Terminology, string, error) {
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, "", err
	}
	if len(concepts) == 0 {
		return nil, "", nil
	}
	mem := terms.NewInMemoryStore()
	h := sha256.New()
	for _, c := range concepts {
		// A concept that does not make it into the snapshot is a term the
		// compliant gate then fails to enforce, silently and for that check
		// only. Nothing here can repair it mid-snapshot, so it is logged and
		// the gate runs on what it has.
		if err := mem.AddConcept(ctx, c); err != nil {
			slog.ErrorContext(ctx, "termcheck: concept omitted from the snapshot; it will not be enforced",
				"concept", c.ID, "error", err)
			continue
		}
		// Every field the predicate reads takes part, because the digest's job
		// is to retire a stored verdict exactly when the governance behind it
		// moved. DoNotTranslate decides whether a target must keep the term at
		// all, and a term's declared forms decide which targets satisfy a rule,
		// so a snapshot differing only in those governs differently.
		fmt.Fprintf(h, "%s|%s|%t|", c.ID, c.Domain, c.DoNotTranslate)
		for _, t := range c.Terms {
			fmt.Fprintf(h, "%s/%s/%s/%t/%s,", t.Locale, t.Text, t.Status, t.CompetitorTerm, strings.Join(t.Forms, "+"))
		}
		h.Write([]byte{'\n'})
	}
	return mem, hex.EncodeToString(h.Sum(nil)), nil
}

// revisionedTerms is a terms store that names the revision of what it holds
// (the PostgreSQL store's PostgresStore.Revision): a token every write through
// it replaces, in the write's own transaction.
type revisionedTerms interface {
	Revision(ctx context.Context) (string, error)
}

// termSnapshotCache keeps each workspace's terms snapshot under the revision
// of the terms it was read at, so the checks that resolve the term gate (a
// commit, the ship and review passes) read a workspace's whole terms once per
// revision rather than once per call. A store that names no revision, or
// names none yet, is read every time.
//
// The revision is read before the terms, so a snapshot is never older than
// the revision it is kept under: a write that lands between the two reads
// leaves a snapshot that already holds it under the revision before it, which
// the next call reads past. Every replica of the server checks the revision
// on every call, so a write through another one is seen at the next call.
//
// It keeps at most termSnapshotWorkspaces snapshots, dropping the one used
// least recently to make room, and drops a workspace's snapshot when its
// revision reads empty (a workspace removed or reset) or cannot be read.
type termSnapshotCache struct {
	mu   sync.Mutex
	byWS map[string]termSnapshot
	// clock orders the snapshots by their last use.
	clock uint64
}

// termSnapshotWorkspaces is the most workspaces whose terms a server keeps a
// snapshot of. A workspace past it reads its terms again on its next call.
const termSnapshotWorkspaces = 32

// termSnapshot is one workspace's snapshot and its digest, read at revision.
type termSnapshot struct {
	revision string
	snap     terms.Terminology
	fp       string
	used     uint64
}

// snapshot returns the snapshot of tb, the terms of workspace wsID, and its
// digest: the one kept for the revision tb holds now, else a fresh read,
// kept when it read cleanly.
func (c *termSnapshotCache) snapshot(ctx context.Context, wsID string, tb terms.Store) (terms.Terminology, string) {
	rt, ok := tb.(revisionedTerms)
	if !ok {
		return snapshotTerms(ctx, tb)
	}
	rev, err := rt.Revision(ctx)
	if err != nil || rev == "" {
		c.drop(wsID)
		return snapshotTerms(ctx, tb)
	}
	c.mu.Lock()
	kept, ok := c.byWS[wsID]
	if ok && kept.revision == rev {
		c.clock++
		kept.used = c.clock
		c.byWS[wsID] = kept
		c.mu.Unlock()
		return kept.snap, kept.fp
	}
	c.mu.Unlock()
	snap, fp, err := readTermSnapshot(ctx, tb)
	if err != nil {
		c.drop(wsID)
		return nil, ""
	}
	c.keep(wsID, termSnapshot{revision: rev, snap: snap, fp: fp})
	return snap, fp
}

// keep keeps s as workspace wsID's snapshot, dropping the snapshot used least
// recently when the cache is full.
func (c *termSnapshotCache) keep(wsID string, s termSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byWS == nil {
		c.byWS = map[string]termSnapshot{}
	}
	if _, held := c.byWS[wsID]; !held && len(c.byWS) >= termSnapshotWorkspaces {
		oldest := ""
		for ws, kept := range c.byWS {
			if oldest == "" || kept.used < c.byWS[oldest].used {
				oldest = ws
			}
		}
		delete(c.byWS, oldest)
	}
	c.clock++
	s.used = c.clock
	c.byWS[wsID] = s
}

// drop forgets workspace wsID's snapshot.
func (c *termSnapshotCache) drop(wsID string) {
	c.mu.Lock()
	delete(c.byWS, wsID)
	c.mu.Unlock()
}
