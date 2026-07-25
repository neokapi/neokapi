package analytics

import "time"

// Server-side event taxonomy (roadmap epic 018, workstreams A/D).
//
// Event names are snake_case domain_action. Every event captured through
// PostHogClient.CaptureEvent automatically carries the mandatory taxonomy
// properties (surface: "server", app_version) and, when a workspace_id
// property is present, the PostHog "workspace" group association.
//
// Every constant defined here MUST be listed in the generated reference doc
// web/docs/contribute/notes-internal/analytics-events.md — the drift gate in
// events_test.go fails otherwise. Events never carry content, file paths, or
// source text.
const (
	// Identity and workspace lifecycle.
	EventUserSignup       = "user_signup"
	EventUserLogin        = "user_login"
	EventWorkspaceCreated = "workspace_created"
	EventProjectCreated   = "project_created"
	EventProjectClaimed   = "project_claimed"
	EventProjectDeleted   = "project_deleted"
	EventMemberInvited    = "member_invited"
	EventMemberJoined     = "member_joined"

	// Domain events (service layer).
	EventFlowRunCompleted   = "flow_run_completed"
	EventContentPushed      = "content_pushed"
	EventContentPulled      = "content_pulled"
	EventReviewApproved     = "review_approved"
	EventReviewRejected     = "review_rejected"
	EventConnectorPublished = "connector_published"

	// Convergence runs (strategy 2026-07-dogfood doc 06, theme D3). A run is
	// correlatable end-to-end by run_id; completed carries the outcome and the
	// machine-readable stall_reason so fleet-wide "where do runs stall" is
	// answerable.
	EventConvergenceRunStarted   = "convergence_run_started"
	EventConvergenceRunCompleted = "convergence_run_completed"
	// EventConvergenceEstimateComputed fires whenever the server computes a
	// pre-flight estimate (the ESTIMATE endpoint), whichever surface asked —
	// the web run-now dialog, `kapi up`'s confirm, or any API client. It is the
	// unbiased complement of the web-only client event
	// convergence_estimate_viewed, and carries the bucketed MAGNITUDE of the
	// work a run would do so the new-workspace credit grant can be sized.
	EventConvergenceEstimateComputed = "convergence_estimate_computed"

	// MCP surface.
	EventMCPSessionStart = "mcp_session_start"
	EventMCPToolCall     = "mcp_tool_call"
	EventMCPResourceRead = "mcp_resource_read"

	// Billing funnel. Normalized from the earlier `billing.*` prefixed names
	// (billing.checkout_started, billing.checkout_completed,
	// billing.feature_gate_hit, billing.credits_exhausted) per the epic-018
	// taxonomy decision; the old names are retired, not dual-fired.
	EventCheckoutStarted       = "checkout_started"
	EventCheckoutCompleted     = "checkout_completed"
	EventTrialStarted          = "trial_started"
	EventTrialConverted        = "trial_converted"
	EventPaymentFailed         = "payment_failed"
	EventSubscriptionCancelled = "subscription_cancelled"
	EventFeatureGateHit        = "feature_gate_hit"
	EventCreditsExhausted      = "credits_exhausted"
)

// Standard property keys and values.
const (
	// PropSurface discriminates the emitting surface in the shared PostHog
	// project; the server always sends SurfaceServer.
	PropSurface = "surface"
	// PropAppVersion carries the server build version.
	PropAppVersion = "app_version"
	// PropWorkspaceID scopes an event to a workspace; a non-empty value also
	// drives the PostHog "workspace" group association ($groups).
	PropWorkspaceID = "workspace_id"
	// PropProjectID scopes an event to a project.
	PropProjectID = "project_id"

	// SurfaceServer is the surface value for all bowrain-server events.
	SurfaceServer = "server"
	// GroupWorkspace is the PostHog group type used for workspace analytics.
	GroupWorkspace = "workspace"
)

// Grant-sizing property keys (strategy: how big must the one-time
// new-workspace grant, billing.FreeTrialGrantCredits, be?). They are shared by
// the estimate events and the run events so the demand side (what a run WOULD
// cost) and the realized side (what it DID cost) join on the same names. Every
// magnitude is bucketed — an event never carries an exact credit amount, token
// count, or unit count.
const (
	// PropEstimatedCreditsBucket buckets the credits a run's AI remainder would
	// spend, per the provider-free pre-flight estimate (CreditBucket).
	PropEstimatedCreditsBucket = "estimated_credits_bucket"
	// PropConsumedCreditsBucket buckets the credits a finished run actually
	// spent, from the token usage its translation jobs recorded (CreditBucket).
	PropConsumedCreditsBucket = "consumed_credits_bucket"
	// PropAIUnitsBucket buckets how many units (blocks) are left for paid AI
	// after TM recycling (CountBucket).
	PropAIUnitsBucket = "ai_units_bucket"
	// PropTMLeveragePctBucket bands the TM share of the work — how much of the
	// pending/produced work TM covers for free (PercentBucket).
	PropTMLeveragePctBucket = "tm_leverage_pct_bucket"
	// PropFirstRun marks the cold-start cohort: true when no earlier convergence
	// run exists for the workspace, i.e. this is its FIRST project run — the
	// cohort the one-time grant has to cover.
	PropFirstRun = "first_run"
	// PropSourceHeld reports whether any source block is held below the gate.
	PropSourceHeld = "source_held"
	// PropCoversAllAI reports whether the workspace balance covers the whole AI
	// remainder of the estimate.
	PropCoversAllAI = "covers_all_ai"
)

// Props builds an event property map carrying the standard workspace/project
// scope. Empty IDs are omitted, so it is safe to call with whatever scope the
// call site has.
func Props(workspaceID, projectID string) map[string]any {
	p := make(map[string]any, 4)
	if workspaceID != "" {
		p[PropWorkspaceID] = workspaceID
	}
	if projectID != "" {
		p[PropProjectID] = projectID
	}
	return p
}

// DurationBucket coarsens a duration into a bucket label so events never
// carry precise timings.
func DurationBucket(d time.Duration) string {
	switch {
	case d < time.Second:
		return "lt_1s"
	case d < 5*time.Second:
		return "1s_5s"
	case d < 30*time.Second:
		return "5s_30s"
	case d < 2*time.Minute:
		return "30s_2m"
	case d < 10*time.Minute:
		return "2m_10m"
	default:
		return "gt_10m"
	}
}

// CountBucket coarsens a count (blocks, segments, items) into a bucket label
// so events never carry exact content sizes.
func CountBucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n <= 10:
		return "1_10"
	case n <= 100:
		return "11_100"
	case n <= 1000:
		return "101_1000"
	default:
		return "gt_1000"
	}
}

// CreditBucket coarsens a credit amount into a bucket label so events never
// carry an exact spend. The edges are anchored on the amounts the pricing model
// already names — the one-time new-workspace grant and one purchased top-up
// pack are both 200_000 credits (billing.FreeTrialGrantCredits /
// billing.CreditPackCredits) — so "what fraction of first runs fits inside the
// grant?" is read straight off the bucket boundary at 200k, and the buckets
// below it say how much slack a smaller grant would still have.
func CreditBucket(credits int64) string {
	switch {
	case credits <= 0:
		return "0"
	case credits <= 1_000:
		return "1_1k"
	case credits <= 10_000:
		return "1k_10k"
	case credits <= 50_000:
		return "10k_50k"
	case credits <= 100_000:
		return "50k_100k"
	case credits <= 200_000:
		return "100k_200k" // upper edge == the current grant / pack size
	case credits <= 500_000:
		return "200k_500k"
	case credits <= 1_000_000:
		return "500k_1m"
	default:
		return "gt_1m"
	}
}

// PercentBucket bands a percentage (0–100) into a coarse label. Out-of-range
// input is clamped, and only an exact 0 / 100 gets the saturated band, so
// "no leverage at all" and "everything was free" stay distinguishable from
// "nearly none" and "nearly all".
func PercentBucket(pct int) string {
	switch {
	case pct <= 0:
		return "0"
	case pct <= 25:
		return "1_25"
	case pct <= 50:
		return "26_50"
	case pct <= 75:
		return "51_75"
	case pct < 100:
		return "76_99"
	default:
		return "100"
	}
}

// SharePercentBucket bands part/whole as a percentage. A zero (or negative)
// whole has no defined share and reads as "0". A non-zero part never collapses
// into the "0" band and a part short of the whole never reaches "100", so the
// saturated bands keep meaning exactly none / exactly all.
func SharePercentBucket(part, whole int) string {
	if whole <= 0 || part <= 0 {
		return "0"
	}
	if part >= whole {
		return "100"
	}
	pct := part * 100 / whole
	if pct <= 0 {
		pct = 1 // a real but tiny share belongs in the lowest non-zero band
	}
	if pct >= 100 {
		pct = 99 // part < whole must not read as "everything"
	}
	return PercentBucket(pct)
}
