// Package review holds the platform's answer to one question: may this user
// decide this unit, at this rung, in this project and this language?
//
// There is one answer because there is one question, and it is asked from
// three places that must not drift apart: the review endpoint a reviewer
// clicks, the bulk route a selection goes through, and the sync worker that
// applies a push. A push moves content; it does not decide. What it carries in
// the way of approvals and sign-offs is held to the same permission and the
// same workspace separation-of-duties policy as a decision made on the
// platform, because otherwise anyone who may write files may also approve
// them, from a laptop, in one command.
//
// The gate resolves what a whole pass needs up front: the workspace policy
// once, the authorship of every (block, locale) pair the pass may touch in one
// query, and each language's permission once. A pass over a thousand blocks
// then costs the same as a pass over one.
package review

import (
	"context"
	"fmt"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/venue"
)

// Refusal is one unit's refused verdict, carrying the reason a surface renders
// and a report counts. It is an error so a caller can pass it up through a
// per-unit path that already speaks in errors, and a typed one so the caller
// can tell a refusal from a failure to ask.
type Refusal struct {
	Reason string
	Locale string
}

func (r Refusal) Error() string { return r.Reason + " for " + r.Locale }

// PolicySource reads a workspace's separation-of-duties policy.
type PolicySource interface {
	GetSoDMode(ctx context.Context, workspaceID string) (platauth.SoDMode, error)
}

// AuthorSource names who last wrote each target by hand. It is
// platstore.TargetAuthorStore, declared here so this package depends on the
// capability rather than on a store.
type AuthorSource interface {
	LastTargetAuthors(ctx context.Context, projectID, stream string, blockIDs, locales []string) (map[platstore.TargetRef]string, error)
}

// Query names one permission question: may this user exercise this permission
// on this language, in this project?
type Query struct {
	ProjectID   string
	WorkspaceID string
	UserID      string
	Permission  platauth.Permission
	Locale      string
}

// Authority answers the permission question for a caller with no request to
// read it from. The web routes read the answer off the request, where the
// access middleware has already resolved it; the sync worker runs long after
// the request is gone and asks an Authority instead.
type Authority interface {
	PolicySource
	// AllowsLanguage reports whether the user holds the permission for the
	// language. An error means the question could not be answered, which is
	// never the same as "no": the caller must refuse to act rather than record
	// a refusal it cannot justify.
	AllowsLanguage(ctx context.Context, q Query) (bool, error)
}

// Config is what one pass needs to open a gate over it.
type Config struct {
	// Actor is the user whose decision is being judged: the reviewer on the
	// web, the authenticated pusher on a push.
	Actor       string
	WorkspaceID string
	ProjectID   string
	Stream      string

	// BlockIDs and Locales bound the authorship query: the cross product is
	// every pair this pass may judge.
	BlockIDs []string
	Locales  []string

	// Policy reads the workspace separation-of-duties mode. Nil leaves the
	// policy off, which is what a deployment with no auth store has.
	Policy PolicySource
	// Authors names the last hand author of each target. Nil is a store that
	// keeps no authorship: it knows of no author, so every pair passes.
	Authors AuthorSource

	// Permits answers the language permission. Nil refuses every language: a
	// gate with no way to ask whether the actor may review must not answer
	// yes.
	Permits func(locale string) bool

	// Record files a separation-of-duties violation. Nil records none, which
	// costs the audit trail rather than the gate.
	Record func(resource string, mode platauth.SoDMode, count int)
	// Silent holds back the per-target violation record, for a pass that files
	// one for the whole run.
	Silent bool
}

// Gate is one pass's answer to the question, with everything the answer needs
// already resolved.
type Gate struct {
	cfg     Config
	mode    platauth.SoDMode
	authors map[platstore.TargetRef]string

	violations int
}

// Open reads the workspace policy and, when it is on, the authorship for the
// pass.
//
// It reports an error only when a store that keeps target authorship failed to
// answer. A discarded error would disable the four-eyes check rather than
// tighten it, so the caller refuses the pass instead. An unreadable policy is
// different: it is not a reason to refuse a review, so it leaves the policy
// off, exactly as a workspace that never set one.
func Open(ctx context.Context, cfg Config) (*Gate, error) {
	g := &Gate{cfg: cfg, mode: platauth.SoDOff}
	if cfg.Actor == "" || cfg.Policy == nil {
		return g, nil
	}
	mode, err := cfg.Policy.GetSoDMode(ctx, cfg.WorkspaceID)
	if err != nil {
		return g, nil // an unreadable policy is not a reason to refuse a review
	}
	g.mode = mode
	if mode == platauth.SoDOff || cfg.Authors == nil || len(cfg.BlockIDs) == 0 || len(cfg.Locales) == 0 {
		return g, nil
	}
	authors, err := cfg.Authors.LastTargetAuthors(ctx, cfg.ProjectID, cfg.Stream, cfg.BlockIDs, cfg.Locales)
	if err != nil {
		return nil, fmt.Errorf("read target authorship for separation of duties: %w", err)
	}
	g.authors = authors
	return g, nil
}

// Quiet stops Allow from filing a violation record per target. A pass over a
// corpus the caller wrote would otherwise file one per block, on a bus that
// drops what it cannot keep up with; the caller files one with the count.
func (g *Gate) Quiet() *Gate {
	if g != nil {
		g.cfg.Silent = true
	}
	return g
}

// Violations is how many pairs the separation-of-duties policy caught, whether
// it blocked them or only recorded them.
func (g *Gate) Violations() int {
	if g == nil {
		return 0
	}
	return g.violations
}

// Mode is the workspace policy this pass is judged under.
func (g *Gate) Mode() platauth.SoDMode {
	if g == nil {
		return platauth.SoDOff
	}
	return g.mode
}

// Allow answers the question for one block's target: may this actor promote it
// to a rung above translated?
//
// Two conditions, in the order a person would ask them. The actor must hold
// review permission for that language in that project. And the workspace
// separation-of-duties policy must pass with the actor as the decider: whoever
// last wrote the translation by hand may not be the one who blesses it, unless
// the workspace has the policy off (nobody is checked) or set to warn (the
// conflict is recorded and allowed).
//
// A pair the store attributes to nobody passes. That is the machine-authored
// case: a draft a run produced was written outside any request, carries no
// acting user, and stays approvable by the one person in a small workspace.
//
// The refusal is a Refusal, so one protected block is recorded against that
// block rather than answering for a whole selection.
func (g *Gate) Allow(blockID, locale string) error {
	if g == nil {
		return Refusal{Reason: venue.RefusedNoReviewPermission, Locale: locale}
	}
	if g.cfg.Permits == nil || !g.cfg.Permits(locale) {
		return Refusal{Reason: venue.RefusedNoReviewPermission, Locale: locale}
	}
	return g.vetSoD(blockID, locale)
}

// AllowWithdrawal answers the other review-level question for one language: may
// this actor lower a target the venue holds at signed-off?
//
// One condition. The actor must hold review permission for that language in
// that project, which is what the web asks before an un-review or a rejection
// drops a signed-off target (HandleReviewBlock's Elevate). The workspace
// separation-of-duties policy is not asked: it judges who may bless work, and
// withdrawing a sign-off blesses nothing, so the author of a translation who
// also holds review may take back their own sign-off here as on the web.
func (g *Gate) AllowWithdrawal(locale string) error {
	if g == nil || g.cfg.Permits == nil || !g.cfg.Permits(locale) {
		return Refusal{Reason: venue.RefusedSignOffWithdrawal, Locale: locale}
	}
	return nil
}

// vetSoD applies the workspace policy to one pair.
func (g *Gate) vetSoD(blockID, locale string) error {
	if g.cfg.Actor == "" || g.mode == platauth.SoDOff {
		return nil
	}
	author := g.authors[platstore.TargetRef{BlockID: blockID, Locale: locale}]
	if author == "" || author != g.cfg.Actor {
		return nil // machine-authored, or somebody else's writing
	}
	g.violations++
	if !g.cfg.Silent && g.cfg.Record != nil {
		g.cfg.Record("approve_block:"+blockID+":"+locale, g.mode, 1)
	}
	if g.mode == platauth.SoDBlock {
		return Refusal{Reason: venue.RefusedSeparationOfDuties, Locale: locale}
	}
	return nil // warn: recorded, but allowed
}

// LanguagePermits builds the per-language predicate a Gate needs from an
// Authority, resolving each language once and remembering the answer.
//
// An unanswerable question is a refusal in the predicate, and the error is kept
// so the caller can tell the two apart: a store that could not say whether the
// pusher may review has not said no, and a caller that treats it as no records
// a refusal it cannot justify. Err reports the first failure.
type LanguagePermits struct {
	ctx   context.Context //nolint:containedctx // the predicate takes only a locale
	auth  Authority
	base  Query
	cache map[string]bool
	err   error
}

// NewLanguagePermits prepares the predicate. A nil Authority permits nothing.
func NewLanguagePermits(ctx context.Context, auth Authority, base Query) *LanguagePermits {
	return &LanguagePermits{ctx: ctx, auth: auth, base: base, cache: map[string]bool{}}
}

// Allows is the predicate a Config.Permits takes.
func (p *LanguagePermits) Allows(locale string) bool {
	if p == nil || p.auth == nil {
		return false
	}
	if allowed, ok := p.cache[locale]; ok {
		return allowed
	}
	q := p.base
	q.Locale = locale
	allowed, err := p.auth.AllowsLanguage(p.ctx, q)
	if err != nil {
		if p.err == nil {
			p.err = fmt.Errorf("resolve review permission for %s: %w", locale, err)
		}
		allowed = false
	}
	p.cache[locale] = allowed
	return allowed
}

// Err reports the first permission lookup that failed, or nil. A caller that
// acts on refusals must check it: a failed lookup is a reason to stop, not a
// reason to refuse.
func (p *LanguagePermits) Err() error {
	if p == nil {
		return nil
	}
	return p.err
}
