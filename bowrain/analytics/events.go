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
