/**
 * Client-side product-analytics event names (roadmap epic 018, workstream B).
 *
 * The single definition site for every event the browser app fires — no
 * scattered string literals. Names are `snake_case` `domain_action`, matching
 * the server-side taxonomy in `bowrain/analytics/events.go`. Every event
 * defined here MUST be listed in the reference doc
 * `web/docs/contribute/notes-internal/analytics-events.md`; the drift test in
 * `src/__tests__/analytics-events.test.ts` fails otherwise (the client-side
 * mirror of the Go drift gate).
 *
 * Events are fired through the AnalyticsContext capture seam
 * (`useAnalytics().capture`), which the web shell backs with PostHog via the
 * platform adapter and the desktop shell leaves unwired (no-op). Events never
 * carry content, free-text values, names, or file paths — ids, locales, and
 * enum-ish values only.
 */
export const AnalyticsEvents = {
  /** A route-derived feature surface was entered ({feature}). */
  featureEntered: "feature_entered",
  /** The create-project form was submitted from the dashboard ({sample}). */
  projectCreateSubmitted: "project_create_submitted",
  /** A connector was added to the workspace ({connector_type}). */
  connectorAdded: "connector_added",
  /** The publish-to-connector confirm was clicked ({connector_category}). */
  connectorPublishClicked: "connector_publish_clicked",
  /** A per-block review decision was clicked ({decision, locale, bulk?}). */
  reviewDecisionClicked: "review_decision_clicked",
  /** A block translation was saved ({locale, method}). */
  translationSaved: "translation_saved",
  /** A workspace settings section was saved ({section}). */
  settingsSaved: "settings_saved",
  /** A member invite was created ({role}). */
  memberInviteSent: "member_invite_sent",
  /** A language was added to the workspace ({locale}). */
  localeAdded: "locale_added",
  /** A brand-voice profile was saved ({mode}). */
  brandVoiceSaved: "brand_voice_saved",
  /** A glossary term status change persisted ({status, locale}). */
  glossarySaved: "glossary_saved",
  /** The locale-demand "Connect PostHog" affordance was clicked ({reconnect}). */
  localeDemandConnectClicked: "locale_demand_connect_clicked",
  /**
   * The run-now consent dialog opened and the pre-flight estimate was shown
   * ({source_held?, covers_all_ai?}) — the source-readiness-first gate (epic
   * 019). Fired before any run is started.
   */
  convergenceEstimateViewed: "convergence_estimate_viewed",
  /**
   * A convergence run was started from the consent dialog ({scope, source_held?})
   * — the user chose a scope (all | ready-only | none) and confirmed (epic 019).
   */
  convergenceRunStarted: "convergence_run_started",
} as const;

export type AnalyticsEventName = (typeof AnalyticsEvents)[keyof typeof AnalyticsEvents];
