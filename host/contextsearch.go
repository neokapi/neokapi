package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	coregraph "github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/occurrence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/review"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// Context retrieval, by content. See AD-037.
//
// One question — *what do we know about this?* — asked once and answered from
// every store the scope holds. Callers do not name a store: which one holds the
// answer is an implementation detail, and making a caller choose forces it to
// know the answer's location before it can ask.
//
// This is the single implementation behind BOTH the `kapi context search` verb
// and the `context_search` MCP tool. The two surfaces are wrappers over this
// function, so they cannot drift — the fragmentation AD-037 records (six CLI
// retrieval verbs, three MCP tools, no rule for which got exposed) came from
// each surface growing its own retrieval path.

// ContextScope names how much of the graph a result set could have come from.
// A caller must be able to tell "this project holds no answer" from "this scope
// cannot hold one", so a narrow scope is reported rather than left implied.
type ContextScope string

const (
	// ScopeProject is a local kapi project: a terms store, a content memory and
	// a profile. No concept relations, no revisions, no market scoping.
	ScopeProject ContextScope = "project"
	// ScopeWorkspace is a Bowrain-connected workspace: the full concept graph.
	ScopeWorkspace ContextScope = "workspace"
	// ScopeProfile is one voice profile and nothing else — a by-name answer with
	// no project in scope, or one naming a profile no recipe declares. Narrower
	// than a project on purpose: there is no terms store and no governance window
	// behind it, and reporting that as an empty project scope would say the
	// project holds no terminology when no project was consulted at all.
	ScopeProfile ContextScope = "profile"
)

// ContextSearchRequest is one question put to the retrieval surface.
type ContextSearchRequest struct {
	Query string
	// Locale narrows term results to a language when set; empty searches all.
	Locale model.LocaleID
	// Limit caps each group independently, not the total — groups are ranked
	// within themselves and never merged, so one shared budget would let a
	// productive group starve the others.
	Limit int
}

// DefaultContextSearchLimit caps each group when a caller names no limit.
const DefaultContextSearchLimit = 10

// ContextSearchResult is the answer, grouped by kind.
//
// Deliberately NOT one ranked list. A term match scores on lexical closeness to
// a concept; a memory match scores on segment similarity. The two numbers are
// not comparable, so blending them would impose an order that means nothing.
// Grouping keeps each ranking honest and lets the caller weigh the kinds.
type ContextSearchResult struct {
	Query string `json:"query"`
	// Scope says how much could have been searched, so an empty result is
	// readable: a project scope that finds nothing has not consulted a concept
	// graph, because it has none.
	Scope ContextScope `json:"scope"`
	// Terms are concepts whose terms or definition match — what the project
	// calls this, and whether it is discouraged.
	Terms []ContextTermHit `json:"terms,omitempty"`
	// Precedent is wording the project has already approved that matches the
	// query — "how have we said this before?".
	//
	// This is NOT translation recycling. Recycling is the loop's job and is
	// invisible by design (`up` pre-fills from content memory as the structural
	// cost control), so offering "search memory for prior translations" would
	// invite a caller to hand-crank work already done. Precedent is a different
	// question: it serves a caller AUTHORING source content who wants its own
	// writing to sound like the project.
	Precedent []ContextPrecedentHit `json:"precedent,omitempty"`
	// Profiles are the governance profiles whose validity is bounded — "which
	// voice is in force, and until when". Only profiles that declare a window
	// appear; a project that never bounds governance has none.
	Profiles []ContextProfileHit `json:"profiles,omitempty"`
	// Notes carries scope-shaped caveats — e.g. that a store the query would
	// have consulted is not bound. Present so "nothing found" is never
	// ambiguous between "no answer" and "nowhere to look".
	Notes []string `json:"notes,omitempty"`
}

// FormatText renders the grouped answer for a human, implementing
// output.TextFormatter. It lives on the shared type rather than in the CLI so
// the text and JSON renderings of a context answer are defined once — the
// surfaces wrap this, they do not each decide what an answer looks like.
func (r *ContextSearchResult) FormatText(w io.Writer) error {
	if len(r.Terms) == 0 && len(r.Precedent) == 0 {
		fmt.Fprintf(w, "Nothing in this project's context matches %q.\n", r.Query)
	}

	if len(r.Terms) > 0 {
		fmt.Fprintln(w, "Terms")
		for _, t := range r.Terms {
			// Lead with the verdict: the caller is asking "may I use this
			// word?", so the answer belongs before the metadata.
			verdict := "ok"
			if t.Discouraged {
				verdict = "discouraged"
				if t.Replacement != "" {
					verdict += `, say "` + t.Replacement + `"`
				}
			}
			fmt.Fprintf(w, "  %-24s %s", t.Term, verdict)
			// Locale before status. One spelling can be deprecated in one
			// language and admitted in another — "termbase" is, in this repo's
			// own terms store — and without the locale those two lines read as
			// the tool contradicting itself rather than answering per language.
			meta := t.Locale
			if t.Status != "" {
				if meta != "" {
					meta += ", "
				}
				meta += t.Status
			}
			if meta != "" {
				fmt.Fprintf(w, " (%s)", meta)
			}
			if t.ValidTo != "" {
				fmt.Fprintf(w, " until %s", validityText(t.ValidTo))
			}
			// The domain is the axis that segments the graph — which part of the
			// subject this concept belongs to. It was carried in the struct and
			// the JSON but never rendered, so the one field that distinguishes
			// one body of context from another was invisible at the surface
			// people actually read.
			if t.Domain != "" {
				fmt.Fprintf(w, "  [%s]", t.Domain)
			}
			fmt.Fprintln(w)
			if t.Definition != "" {
				fmt.Fprintf(w, "  %-24s %s\n", "", t.Definition)
			}
			// Where it is used, if anywhere. A discouraged word with no uses
			// is a settled question; the same word in thirty blocks is work.
			if t.Uses > 0 {
				fmt.Fprintf(w, "  %-24s used %d time(s) in %d block(s)\n", "", t.Uses, t.UseBlocks)
				for _, u := range t.TopUses {
					where := u.Document
					if u.BlockID != "" {
						where += " " + u.BlockID
					}
					// The point the file is governed at, when the recipe binds
					// one: the same word can be admitted on one surface and
					// discouraged on another, so "where" means the coordinate as
					// much as the path.
					if u.Point != "" {
						where += " @" + u.Point
					}
					if u.Locale != "" {
						where += " [" + u.Locale + "]"
					}
					fmt.Fprintf(w, "  %-24s   %s: %s\n", "", strings.TrimSpace(where), u.Snippet)
				}
			}
		}
	}

	if len(r.Precedent) > 0 {
		fmt.Fprintln(w, "\nAlready approved wording")
		for _, p := range r.Precedent {
			fmt.Fprintf(w, "  %s\n", p.Text)
			if len(p.Discouraged) > 0 {
				fmt.Fprintf(w, "  %-24s uses retired wording: %s\n", "",
					strings.Join(p.Discouraged, ", "))
			}
		}
	}

	if len(r.Profiles) > 0 {
		fmt.Fprintln(w, "\nGoverning profiles")
		for _, p := range r.Profiles {
			window := ""
			if p.ValidFrom != "" {
				window += "from " + p.ValidFrom + " "
			}
			if p.ValidTo != "" {
				window += "until " + p.ValidTo
			}
			window = strings.TrimSpace(window)
			line := "  " + p.Name
			if window != "" {
				line += "  " + window
			}
			fmt.Fprintf(w, "%s (%s)\n", line, p.State)
		}
	}

	// Notes last and always: they are what makes an empty or partial answer
	// readable rather than ambiguous.
	if len(r.Notes) > 0 {
		fmt.Fprintln(w)
		for _, n := range r.Notes {
			fmt.Fprintf(w, "  note: %s\n", n)
		}
	}
	return nil
}

// ContextTermHit is one concept the query matched.
type ContextTermHit struct {
	ConceptID  string `json:"concept_id"`
	Term       string `json:"term"`
	Locale     string `json:"locale,omitempty"`
	Status     string `json:"status,omitempty"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
	// Replacement is what to say instead, when the matched term is discouraged.
	// Derived, not stored: a concept groups its terms per locale, so the
	// replacement for a deprecated word is the preferred word for the same
	// concept in the same language. A caller asking "may I use this?" needs the
	// answer and the alternative together, not a second lookup.
	Replacement string `json:"replacement,omitempty"`
	// Discouraged is the one-glance answer to "may I use this word?", derived
	// from Status so a caller does not have to know the status vocabulary.
	Discouraged bool `json:"discouraged"`
	// ValidFrom/ValidTo carry the term's own time scoping when it has any.
	// A rule that starts or stops on a date is what lets a context survive a
	// rename without being rewritten, so a caller must be able to see that the
	// answer it just got has an expiry — "recognise the old name until March"
	// is a different instruction from "never use it".
	ValidFrom string `json:"valid_from,omitempty"`
	ValidTo   string `json:"valid_to,omitempty"`
	// Uses is how many times this exact term appears in the project's
	// extracted content, and UseBlocks in how many blocks. Together they turn
	// "this word is discouraged" into "this word is discouraged and sits in 34
	// places", which is the difference between a rule and a job. Zero when no
	// block cache is bound — see the note the search adds in that case.
	Uses      int `json:"uses,omitempty"`
	UseBlocks int `json:"use_blocks,omitempty"`
	// TopUses are the first few of those uses, enough to see what kind of
	// content is involved without turning a context answer into a report.
	// `kapi terms occurrences` is where the full list lives.
	TopUses []ContextTermUse `json:"top_uses,omitempty"`
}

// ContextTermUse is one place a term is used, as a context answer shows it.
type ContextTermUse struct {
	Document string `json:"document,omitempty"`
	BlockID  string `json:"block_id,omitempty"`
	Locale   string `json:"locale,omitempty"`
	Snippet  string `json:"snippet,omitempty"`
	// Point is the place in the context space this document sits at, written
	// `profile/channel`, resolved per file so a content item's own `channel:`
	// shows where its files are actually governed. Empty when the document sits
	// at the project's default point, or when no recipe was bound to resolve
	// against.
	Point string `json:"point,omitempty"`
}

// ContextPrecedentHit is one piece of previously-approved wording.
type ContextPrecedentHit struct {
	EntryID string `json:"entry_id"`
	Text    string `json:"text"`
	Locale  string `json:"locale,omitempty"`
	Note    string `json:"note,omitempty"`
	// Discouraged names terms in this wording that the same search found to be
	// retired. Approval is a fact about when the wording was written, not a
	// standing endorsement: terminology moves underneath content, and that gap
	// is exactly what a rename produces. Offering such a line unqualified as
	// "already approved" would make one answer contradict itself.
	Discouraged []string `json:"discouraged,omitempty"`
}

// ContextProfileHit is a governance profile whose validity is bounded, reported
// so an answer can say which voice is in force and until when. It is the review
// model's own row (core/review.ProfileValidity): the same fact, read once.
type ContextProfileHit = review.ProfileValidity

// ContextSearchSources are the stores a search consults. A nil member is a
// store the caller has not bound; the search proceeds over the rest and says so
// in Notes rather than failing, because a project with terms but no content
// memory is ordinary, not broken.
type ContextSearchSources struct {
	Terms  terms.Terminology
	Memory memory.Store
	// Blocks is the project's extracted content. It answers a different kind
	// of question from the other two — not "what do we know about this word?"
	// but "where is it?" — and it is bound here rather than asked separately
	// because the two belong in one answer: a caller told a term is
	// discouraged needs to know at once whether anything uses it.
	Blocks blockstore.Store
	Scope  ContextScope
	// TermsErr and MemoryErr are set when the caller tried to bind that store
	// and could not. Unbound and unopenable are different answers to "why is
	// this group empty?", and only the caller knows which happened.
	//
	// Typed per store rather than a free-text note list so the two cannot be
	// reported at once: a store that failed to open is not also a store the
	// project does not have. The SQLite openers create a missing file rather
	// than reporting absence, so an error from one always means broken —
	// dropping it, as both surfaces originally did, told the caller its project
	// had no terminology when in fact it could not be read.
	TermsErr  error
	MemoryErr error

	// Profiles are the project's bounded governance profiles — surfaced so an
	// answer can say which voice is in force and until when. Populated from the
	// resolved recipe; empty for a project that bounds no profile, or a
	// standalone-store query with no project in scope.
	Profiles []ContextProfileHit

	// Freshness carries staleness notes about the graph these stores are read
	// from: governance that moved since this process last read it. Resolved by
	// the caller (host/freshness.go) because it is a property of the reader's
	// history, not of any store — the same stores read by a fresh process are
	// not stale, they are simply unread.
	Freshness []string

	// Unseeded reports a project whose committed context sources have never been
	// compiled into the store this search reads — a fresh clone, before anything
	// ran. Its stores answer, and answer empty, which is indistinguishable from
	// a project that genuinely holds no such term. The by-location answer reads
	// the committed documents directly and does not have this state, so leaving
	// it unsaid is what let the two retrieval primitives disagree in silence.
	Unseeded bool

	// Recipe is the project whose context space the answer resolves places
	// against, so a term's uses can say where each one is governed. nil for a
	// standalone-store query with no project in scope.
	Recipe *project.KapiProject
	// At is the instant governance is resolved at — the run's wall clock. The
	// zero value reads the recipe as declared, applying no validity window.
	At time.Time
}

// ContextSearchSourcesFor assembles the stores a context search reads — the one
// path for both `kapi context search` and the context_search MCP tool, so the
// two cannot drift in what they bind. A non-empty termsPath/memoryPath selects a
// STANDALONE store (an agent pointed at a vocabulary or corpus outside the
// project); otherwise the store the command resolves to is used: the project's
// own store, or a --name/--file/--local selection on the CLI. The MCP half
// passes a bare synthetic command, so it takes the project defaults.
//
// A store that will not open degrades to a note (TermsErr/MemoryErr) rather than
// failing, since the other store may still answer. The returned cleanup releases
// any store opened here; run it after SearchContext.
func (a *App) ContextSearchSourcesFor(cmd Command, termsPath, memoryPath string) (ContextSearchSources, func()) {
	src := ContextSearchSources{Scope: ScopeProject}
	var cleanups []func()

	if termsPath != "" {
		if tb, err := terms.NewSQLiteStore(termsPath); err == nil {
			cleanups = append(cleanups, func() { _ = tb.Close() })
			src.Terms = tb
		} else {
			src.TermsErr = err
		}
	} else if tb, _, release, err := a.OpenTermsSQLite(cmd); err == nil {
		cleanups = append(cleanups, release)
		src.Terms = tb
	} else {
		src.TermsErr = err
	}

	if memoryPath != "" {
		if tm, err := memory.NewSQLiteStore(memoryPath); err == nil {
			cleanups = append(cleanups, func() { _ = tm.Close() })
			src.Memory = tm
		} else {
			src.MemoryErr = err
		}
	} else if tm, _, release, err := a.OpenMemorySQLite(cmd); err == nil {
		cleanups = append(cleanups, release)
		src.Memory = tm
	} else {
		src.MemoryErr = err
	}

	// The project's extracted content, so a term answer can say where the term
	// actually is. OccurrenceBlocks already prefers an injected BlocksBackend
	// (the browser build) over the file-backed store.
	src.Blocks = a.OccurrenceBlocks(cmd)

	// The recipe: its bounded governance profiles, so an answer can say which
	// voice is in force and until when, and the context space itself, so each
	// place a term is used can say where it is governed. Best-effort — a project
	// that will not resolve or load simply contributes neither, exactly like a
	// standalone-store query.
	src.At = a.GovernanceInstant()
	if path, err := ResolveProjectPath(cmd); err == nil && path != "" {
		if proj, err := project.Load(path); err == nil {
			src.Recipe = proj
			src.Profiles = profileHits(proj.ProfileWindows(), src.At)
			src.Unseeded = a.ContextSourcesUnseeded(ctxOrBackground(cmd.Context()), path)
		}
	}

	// Whether the graph these stores hold has moved since this process last
	// read it. Assembled here, with the stores, so every surface that retrieves
	// context reports staleness identically — a note that appeared on one
	// surface and not the other would teach an assistant that only one of them
	// can go stale.
	src.Freshness = a.governanceNotes(cmd)

	return src, func() {
		for _, c := range cleanups {
			c()
		}
	}
}

// SearchContext answers one context question from every bound store.
//
// A store that errors degrades to a note rather than failing the whole call:
// half an answer plus a statement of what was unreachable is more useful to a
// caller mid-task than an error, and silently dropping the failure would make
// an incomplete answer look complete — the exact defect AD-037 records in the
// surface this replaces.
func SearchContext(ctx context.Context, src ContextSearchSources, req ContextSearchRequest) (*ContextSearchResult, error) {
	if req.Query == "" {
		return nil, errors.New("context search: empty query")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultContextSearchLimit
	}
	scope := src.Scope
	if scope == "" {
		scope = ScopeProject
	}

	res := &ContextSearchResult{Query: req.Query, Scope: scope}

	// Freshness leads the notes. A caller that reads one note reads this one:
	// it is the only note that says the rest of the answer may already be
	// answering a question about a graph that has moved.
	res.Notes = append(res.Notes, src.Freshness...)
	if src.Unseeded {
		res.Notes = append(res.Notes,
			"this project's committed context has not been compiled into its store yet, so an empty answer here means unread rather than absent. Run `kapi up`")
	}

	if src.Terms != nil {
		concepts, _, err := src.Terms.Search(ctx, req.Query, req.Locale, "", 0, limit)
		if err != nil {
			res.Notes = append(res.Notes, "terms store could not be searched: "+err.Error())
		} else {
			res.Terms = termHits(concepts, req.Locale)
		}
	} else if src.TermsErr != nil {
		res.Notes = append(res.Notes, "terms store could not be opened: "+src.TermsErr.Error())
	} else {
		res.Notes = append(res.Notes, "no terms store is bound, so terminology was not consulted")
	}

	if src.Memory != nil {
		entries, _, err := src.Memory.SearchEntries(ctx, memory.SearchParams{
			Query: req.Query,
			Limit: limit,
		})
		if err != nil {
			res.Notes = append(res.Notes, "content memory could not be searched: "+err.Error())
		} else {
			res.Precedent = precedentHits(entries, req.Locale, limit)
		}
	} else if src.MemoryErr != nil {
		res.Notes = append(res.Notes, "content memory could not be opened: "+src.MemoryErr.Error())
	} else {
		res.Notes = append(res.Notes, "no content memory is bound, so prior wording was not consulted")
	}

	res.Profiles = src.Profiles

	flagRetiredPrecedent(res.Terms, res.Precedent)
	res.Notes = append(res.Notes, countTermUses(ctx, src, res.Terms)...)

	if scope == ScopeProject {
		res.Notes = append(res.Notes,
			"project scope: concept relations, revisions and market scoping live in a connected workspace")
	}

	return res, nil
}

// profileHits renders the project's bounded governance profiles for a context
// answer, reading each window against now so the caller sees at a glance whether
// a profile is active, not yet in force, or expired.
func profileHits(windows []project.ProfileWindow, at time.Time) []ContextProfileHit {
	if len(windows) == 0 {
		return nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	out := make([]ContextProfileHit, 0, len(windows))
	for _, w := range windows {
		hit := ContextProfileHit{Name: w.Name, State: validityState(w.Validity, at)}
		if w.Validity != nil {
			hit.ValidFrom = project.FormatValidityBound(w.Validity.ValidFrom)
			hit.ValidTo = project.FormatValidityBound(w.Validity.ValidTo)
		}
		out = append(out, hit)
	}
	return out
}

// validityText renders a bound a hit carries as RFC3339 the way a reader writes
// it: a bare date when it falls on midnight UTC, the full instant otherwise.
// The stored form stays RFC3339 — a program reading the JSON wants one shape —
// and only the prose is shortened, by both answers, so a date reads the same
// wherever it appears.
func validityText(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return project.FormatValidityBound(&t)
}

// validityState reads a window against an instant: expired once ValidTo has
// passed, upcoming before ValidFrom, active in between (and for an unbounded
// window).
func validityState(v *coregraph.Validity, at time.Time) string {
	if v == nil {
		return "active"
	}
	if v.ValidTo != nil && !at.Before(*v.ValidTo) {
		return "expired"
	}
	if v.ValidFrom != nil && at.Before(*v.ValidFrom) {
		return "upcoming"
	}
	return "active"
}

// contextTopUses is how many places a context answer names per term. A context
// answer is a briefing, not a report: enough to see what kind of content is
// involved, and `kapi terms occurrences` for the rest.
const contextTopUses = 3

// countTermUses fills in where each matched term is actually used, and returns
// any note the attempt produced.
//
// One query per concept, not per hit: a concept's terms are looked up together
// and the results handed to the hits that asked for them, so a concept with six
// terms costs one search rather than six.
func countTermUses(ctx context.Context, src ContextSearchSources, hits []ContextTermHit) []string {
	if len(hits) == 0 {
		return nil
	}
	if src.Blocks == nil {
		return []string{"no block cache is bound, so term usage was not counted. Run `kapi up` to extract content"}
	}

	byConcept := map[string][]int{}
	for i, h := range hits {
		byConcept[h.ConceptID] = append(byConcept[h.ConceptID], i)
	}

	places := &pointResolver{proj: src.Recipe, at: src.At, cache: map[string]string{}}
	var notes []string
	for conceptID, idx := range byConcept {
		res, err := occurrence.Find(ctx, occurrence.Sources{Terms: src.Terms, Blocks: src.Blocks},
			occurrence.Query{Subject: conceptID})
		if err != nil {
			// An unknown concept here would mean the terms store changed under
			// the search; anything else is worth saying once.
			if !errors.Is(err, occurrence.ErrUnknownSubject) {
				notes = append(notes, "term usage could not be counted: "+err.Error())
			}
			continue
		}
		for _, i := range idx {
			attachUses(&hits[i], res.Occurrences, places)
		}
	}
	notes = append(notes, places.notes...)
	sort.Strings(notes)
	return notes
}

// attachUses gives one hit the occurrences of its own term. Matching on the
// term text keeps a per-language answer per-language: a concept's Norwegian
// term is used where the Norwegian text uses it, not wherever the concept is.
func attachUses(hit *ContextTermHit, occurrences []occurrence.Occurrence, places *pointResolver) {
	blocks := map[string]bool{}
	for _, o := range occurrences {
		if !strings.EqualFold(o.Term, hit.Term) {
			continue
		}
		hit.Uses++
		blocks[o.BlockHash] = true
		if len(hit.TopUses) < contextTopUses {
			hit.TopUses = append(hit.TopUses, ContextTermUse{
				Document: o.Document,
				BlockID:  o.BlockID,
				Locale:   o.Locale,
				Snippet:  o.Snippet,
				Point:    places.pointOf(o.Document),
			})
		}
	}
	hit.UseBlocks = len(blocks)
}

// pointResolver answers "where is this document governed" for the places a term
// is used, through the one resolution seam every other surface uses — so the
// answer a writer reads and the voice a run applies to the same file come from
// the same walk of the recipe, including a content item's own `channel:` and a
// profile that has stopped governing.
type pointResolver struct {
	proj  *project.KapiProject
	at    time.Time
	cache map[string]string
	notes []string
	noted map[string]bool
}

// pointOf renders the point a document sits at as `profile/channel`, or "" for
// the project's default point. A validity transition applied on the way is
// carried into the answer's notes once, so a reader is never told a rule is in
// force by a search that just watched it lapse.
func (r *pointResolver) pointOf(document string) string {
	if r == nil || r.proj == nil || document == "" {
		return ""
	}
	if p, ok := r.cache[document]; ok {
		return p
	}
	rc, err := r.proj.ResolveGovernanceFor(project.GovernancePoint{Path: document, At: r.at})
	if err != nil {
		r.cache[document] = ""
		return ""
	}
	if rc.Fallback != nil {
		if r.noted == nil {
			r.noted = map[string]bool{}
		}
		if msg := rc.Fallback.String(); !r.noted[msg] {
			r.noted[msg] = true
			r.notes = append(r.notes, msg)
		}
	}
	point := rc.Ref().String()
	r.cache[document] = point
	return point
}

// flagRetiredPrecedent marks prior wording containing a term this same search
// found discouraged.
//
// Without it a single answer contradicts itself. That is not hypothetical: the
// first time this surface was pointed at this repo's own context, a search for
// "termbase" reported the word retired and then offered "Export termbase to
// CSV" under "already approved wording".
//
// Scoped to the terms in THIS result rather than every retired term in the
// store: checking all content against all terminology is the terms check tool's
// job, and duplicating it here would make one question's answer depend on the
// whole store's state instead of on what was asked.
func flagRetiredPrecedent(hits []ContextTermHit, precedent []ContextPrecedentHit) {
	retired := make(map[string]string) // lowercase form -> term as written
	for _, t := range hits {
		if t.Discouraged {
			retired[strings.ToLower(t.Term)] = t.Term
		}
	}
	if len(retired) == 0 {
		return
	}
	for i := range precedent {
		text := strings.ToLower(precedent[i].Text)
		for lower, term := range retired {
			if strings.Contains(text, lower) {
				precedent[i].Discouraged = append(precedent[i].Discouraged, term)
			}
		}
		// Map iteration is unordered; a rendered answer must not reshuffle
		// between identical runs.
		sort.Strings(precedent[i].Discouraged)
	}
}

// termHits projects concepts into the flat, answer-shaped hits a caller wants:
// what this is called, whether it is discouraged, and what to say instead.
func termHits(concepts []terms.Concept, locale model.LocaleID) []ContextTermHit {
	var out []ContextTermHit
	for _, c := range concepts {
		for _, t := range c.Terms {
			if locale != "" && t.Locale != locale {
				continue
			}
			hit := ContextTermHit{
				ConceptID:   c.ID,
				Term:        t.Text,
				Locale:      string(t.Locale),
				Status:      string(t.Status),
				Definition:  c.Definition,
				Domain:      c.Domain,
				Discouraged: t.Status.Discouraged(),
			}
			if hit.Discouraged {
				hit.Replacement = preferredTerm(c, t.Locale)
			}
			if v := t.Validity; v != nil {
				if v.ValidFrom != nil {
					hit.ValidFrom = v.ValidFrom.UTC().Format(time.RFC3339)
				}
				if v.ValidTo != nil {
					hit.ValidTo = v.ValidTo.UTC().Format(time.RFC3339)
				}
			}
			out = append(out, hit)
		}
	}
	return out
}

// preferredTerm finds the concept's preferred wording in one locale — what to
// say instead of a discouraged term. Returns "" when the concept records no
// preference, which is a real state: a term can be banned without an anointed
// replacement, and inventing one would be worse than saying nothing.
func preferredTerm(c terms.Concept, locale model.LocaleID) string {
	for _, t := range c.Terms {
		if t.Locale == locale && t.Status == model.TermPreferred {
			return t.Text
		}
	}
	return ""
}

// precedentHits projects memory entries into approved wording. Each entry is
// multilingual; the requested locale wins, otherwise the entry's source-language
// hint, so a caller authoring in one language is not handed another's text.
func precedentHits(entries []memory.Entry, locale model.LocaleID, limit int) []ContextPrecedentHit {
	var out []ContextPrecedentHit
	for _, e := range entries {
		if len(out) >= limit {
			break
		}
		pick := locale
		if pick == "" {
			pick = e.HintSrcLang
		}
		runs, ok := e.Variants[pick]
		if !ok {
			// No variant in the wanted language: this entry has nothing to say
			// to this caller, and offering another language's text as precedent
			// would be actively misleading.
			continue
		}
		out = append(out, ContextPrecedentHit{
			EntryID: e.ID,
			Text:    model.RunsText(runs),
			Locale:  string(pick),
			Note:    e.Note,
		})
	}
	return out
}
