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
  /**
   * A terms status change persisted ({status, locale}).
   *
   * The event id stays `glossary_saved`. An analytics id is a join key into
   * history: renaming it starts a new series and silently truncates every chart,
   * funnel, and saved query built on the old one. The vocabulary sweep that
   * renamed "glossary"/"termbase" to "terms" elsewhere deliberately left this
   * alone — rename it only alongside a dashboard migration that unions both
   * spellings across the cutover.
   */
  glossarySaved: "glossary_saved",
  /** The locale-demand "Connect PostHog" affordance was clicked ({reconnect}). */
  localeDemandConnectClicked: "locale_demand_connect_clicked",
  /**
   * The run-now consent dialog opened and the pre-flight estimate was shown
   * ({source_held?, covers_all_ai?, estimated_credits_bucket, ai_units_bucket,
   * tm_leverage_pct_bucket}) — the source-readiness-first gate (epic 019).
   * Fired before any run is started.
   *
   * The bucketed magnitudes (see `analytics-buckets.ts`) are what size the
   * one-time new-workspace credit grant; this dialog-only sample is biased
   * toward the web app, so the server fires `convergence_estimate_computed`
   * for every pre-flight it computes, `kapi up` included.
   */
  convergenceEstimateViewed: "convergence_estimate_viewed",
  /**
   * A convergence run was started from the consent dialog ({scope, source_held?})
   * — the user chose a scope (all | ready-only | none) and confirmed (epic 019).
   */
  convergenceRunStarted: "convergence_run_started",
  /**
   * The GitHub App setup page loaded from a GitHub redirect
   * (`setup_action` present) but WITHOUT an installation id
   * ({setup_action}) — the post-install/update handoff lost the id, so
   * the user landed on the recovery card instead of the repo list.
   */
  githubSetupInstallationMissing: "github_setup_installation_missing",
} as const;

export type AnalyticsEventName = (typeof AnalyticsEvents)[keyof typeof AnalyticsEvents];
