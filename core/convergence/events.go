package convergence

// EventType discriminates the progress events of a convergence run.
type EventType string

// Event types, in the order a run emits them. One run is a sequence of
// passes; within a pass every pending locale runs the default flow (possibly
// concurrently), and after the pass coverage is re-derived with the project's
// bound checks. The stream ends with exactly one EventDone.
const (
	// EventPassStart opens pass Pass (1-based, capped at MaxPasses) over the
	// Pending locales. ExtractedFiles/ExtractedBlocks report the pre-pass
	// auto-extract on block-store drift (both zero when in sync).
	EventPassStart EventType = "pass_start"
	// EventLocaleStart reports one locale's flow run beginning inside the
	// current pass; Units is the locale's total unit count (the denominator a
	// progress bar renders against).
	EventLocaleStart EventType = "locale_start"
	// EventUnitProgress is the throttled live counter for one locale: Done
	// units carry a committed target so far, of which ViaMemory came from content memory
	// recycling and ViaAI from an AI/MT engine.
	EventUnitProgress EventType = "unit_progress"
	// EventLocaleDone reports one locale's flow run finishing inside the
	// current pass, with the final Done/ViaMemory/ViaAI counts for the pass.
	EventLocaleDone EventType = "locale_done"
	// EventPassDone closes a pass after post-derivation: Produced units at
	// ≥ draft, the pass's ProducedDelta, FailingChecks demoted by the bound
	// checks, and the locales still Pending (short of their gate).
	EventPassDone EventType = "pass_done"
	// EventMaterialized reports the post-loop materialize step: Files written
	// across every shippable locale.
	EventMaterialized EventType = "materialized"
	// EventLog carries a human-readable run log line (auto-extract notes,
	// venue transport notes) that has no structured shape of its own.
	EventLog EventType = "log"
	// EventDone closes the stream: State is "converged" or "parked".
	EventDone EventType = "done"
)

// eventTypes is the closed set of progress events a run emits.
var eventTypes = map[EventType]bool{
	EventPassStart:    true,
	EventLocaleStart:  true,
	EventUnitProgress: true,
	EventLocaleDone:   true,
	EventPassDone:     true,
	EventMaterialized: true,
	EventLog:          true,
	EventDone:         true,
}

// KnownEventType reports whether t names a progress event.
//
// A reader of a mixed NDJSON document uses it to tell a run's events from the
// other records that share the stream's `type` key: it forwards what it
// recognises and leaves the rest to the reader that owns it, so a record added
// beside the events (a proposed change-set, a pushed voice profile) never
// reaches a run view as an event with every field zero.
func KnownEventType(t EventType) bool { return eventTypes[t] }

// Locale states reported by EventLocaleDone / the final standing.
const (
	LocaleShippable = "shippable"
	LocaleParked    = "parked"
	LocalePending   = "pending"
)

// Run states reported by EventDone. A run ends converged (every gated scope
// shippable) or parked (work remains for a human) on the happy path; failed
// (the loop errored — e.g. a provider outage) and canceled (stopped by a user
// or a server restart) are terminal error states that must be distinguishable
// so a caller does not mistake a broken run for parked work and exit 0.
const (
	RunConverged = "converged"
	RunParked    = "parked"
	RunFailed    = "failed"
	RunCanceled  = "canceled"
)

// Stage names the loop phase an event was emitted from — the venue-neutral
// vocabulary a surface renders as "where in the loop the run is" (strategy
// 2026-07-dogfood doc 06, theme D). The Event.Stage field is optional: a
// consumer that predates it ignores an empty Stage, and a venue that does not
// distinguish phases may leave it unset. sync/derive are loop-owned;
// recycle/ai_translate/checks/materialize are what a venue's Produce and finish
// steps report.
const (
	StageSync         = "sync"          // pre-pass source sync / drift re-extract
	StageSettleSource = "settle_source" // source-settlement phase before target production (source-first, epic 019)
	StageDerive       = "derive"        // coverage + bound-check derivation
	StageRecycle      = "recycle"       // Memory-leverage stage of production
	StageAITranslate  = "ai_translate"  // AI/MT drafting stage of production
	StageChecks       = "checks"        // post-production checks
	StageMaterialize  = "materialize"   // writing shippable output
)

// StallReason is the machine-readable cause a run did not converge — the label
// that turns a silent stuck spinner into an actionable state (strategy
// 2026-07-dogfood doc 06, theme C). It is set on the run row and carried on the
// terminal done event so the UI and analytics can distinguish, say, "pending
// human review" from a provider outage. The type is an open string vocabulary:
// a host may record reasons of its own (a platform, for instance, adds a
// credit-exhaustion reason) without extending this list.
type StallReason = string

const (
	// StallNone is the zero value: the run converged, or has no blocking
	// reason recorded.
	StallNone StallReason = ""
	// StallNeedsAIKey: no usable AI provider/key (provider or auth error).
	StallNeedsAIKey StallReason = "needs_ai_key"
	// StallRateLimited: the provider returned repeated 429s.
	StallRateLimited StallReason = "rate_limited"
	// StallNoProgress: a full pass produced nothing new and the remainder
	// cannot advance unaided — ordinary parking (genuine pending work).
	StallNoProgress StallReason = "no_progress"
	// StallChecksFailing: coverage is complete but bound checks demote units
	// below the gate, so the locale parks on failing terminology/length checks.
	StallChecksFailing StallReason = "checks_failing"
	// StallSourceNotReady: the source itself is below the source-first gate
	// (terminology/voice/source hygiene not settled, or human source review pending),
	// so the fan-out is HELD on source rather than translating an unsettled,
	// non-compliant source into N locales (strategy 2026-07-dogfood doc 07 / roadmap
	// epic 019). The run creates a source-review task and parks; settling the
	// source (or lowering `defaults.source_gate`) lets the next run translate.
	StallSourceNotReady StallReason = "source_not_ready"
	// StallNoTargetLocales: a project the venue holds for its per-language work
	// has none to do, because it names no target language. It is a configuration
	// hold rather than an up-to-date state: a run over N source blocks with zero
	// target locales must never read "converged" at a venue whose whole purpose
	// is the fan-out.
	//
	// It is a SERVER-venue reason. The local venue reads the same recipe as
	// monolingual — the front door, where one language is the answer rather than
	// a missing setting — and reconciles the source alone, reporting the run as
	// monolingual instead of stalled.
	StallNoTargetLocales StallReason = "no_target_locales"
)

// Event is one progress event of a convergence run — the single protocol every
// venue and surface speaks: the CLI's live renderer, `kapi up --json` (NDJSON,
// one event per line), the desktop's run view, and a Bowrain server run's SSE
// stream all carry exactly this shape, so nothing about rendering a run knows
// where it executes.
//
// The struct is deliberately flat with a Type discriminator (not an interface)
// so it serializes identically over JSON lines, SSE, and the desktop's event
// bridge. Fields are populated per Type as documented on the EventType
// constants; unused fields stay zero and are omitted from JSON.
type Event struct {
	Type EventType `json:"type"`

	// Stage is the loop phase this event was emitted from
	// (sync|derive|recycle|ai_translate|checks|materialize). Optional and
	// back-compatible: a consumer may ignore it, and it is omitted from JSON
	// when unset. It gives a surface "where in the loop the run is" without
	// inferring it from the event type (theme D).
	Stage string `json:"stage,omitempty"`

	// Pass-scoped fields (pass_start, pass_done; Pass also stamps every
	// locale-scoped event so consumers need no ambient state).
	Pass      int      `json:"pass,omitempty"`
	MaxPasses int      `json:"maxPasses,omitempty"`
	Pending   []string `json:"pending,omitempty"`

	// Pre-pass auto-extract on drift (pass_start).
	ExtractedFiles  int `json:"extractedFiles,omitempty"`
	ExtractedBlocks int `json:"extractedBlocks,omitempty"`

	// Locale-scoped fields (locale_start, unit_progress, locale_done).
	Locale    string `json:"locale,omitempty"`
	Units     int    `json:"units,omitempty"`
	Done      int    `json:"done,omitempty"`
	ViaMemory int    `json:"viaTM,omitempty"`
	ViaAI     int    `json:"viaAI,omitempty"`

	// Post-derivation fields (pass_done).
	Produced      int `json:"produced,omitempty"`
	ProducedDelta int `json:"producedDelta,omitempty"`
	FailingChecks int `json:"failingChecks,omitempty"`

	// Source-first fields (settle_source stage / pass_done / done). SettledSource
	// is how many source blocks the settlement phase stamped this pass;
	// BlockedOnSource is how many remain below the source gate — the count the UI
	// renders as "N segments need source review before translating" and the
	// signal that a run held on source (source_not_ready). Both are omitted when
	// zero, so a project with no source gate (or a fully-settled source) carries
	// neither (strategy 2026-07-dogfood doc 07 / roadmap epic 019).
	SettledSource   int `json:"settledSource,omitempty"`
	BlockedOnSource int `json:"blockedOnSource,omitempty"`

	// State on done is the run outcome (converged|parked|failed|canceled) and
	// is always set. On locale_done State is OPTIONAL: a per-locale
	// shippable|parked|pending verdict is a whole-pass property (it depends on
	// the gate rollup after every locale finishes), so the live stream may
	// leave it empty and a consumer should take authoritative per-locale state
	// from the run's final standing, not from a streamed locale_done.
	State string `json:"state,omitempty"`

	// StallReason (done) is the machine-readable cause a run did not converge:
	// needs_ai_key | rate_limited | no_progress | checks_failing (an open
	// vocabulary; hosts may add their own). Empty on a converged run (or when no
	// reason is recorded), so an actionable stall is distinguishable from a
	// clean finish (theme C).
	StallReason StallReason `json:"stallReason,omitempty"`

	// Materialized file count (materialized).
	Files int `json:"files,omitempty"`

	// Log line (log).
	Message string `json:"message,omitempty"`
}
