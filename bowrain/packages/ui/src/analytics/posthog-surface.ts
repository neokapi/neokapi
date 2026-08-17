// Shared PostHog surface initializer for the Bowrain SPAs (web, ctrl, pulse).
//
// Every event in the shared PostHog project must carry two taxonomy
// super-properties — `surface` (which app) and `environment` — so metrics can
// be split by app and by prod/preview/dev. Registering them lived in each app's
// own init, and drifted: the ctrl and pulse copies dropped `environment`
// entirely. This helper owns the key-gating and the mandatory register() so no
// app re-remembers them.
//
// Typed against a structural PostHog subset rather than importing posthog-js, so
// @neokapi/ui takes on no analytics dependency; each app passes its own client
// and keeps its own init-once guard.

/** The subset of the posthog-js client this initializer drives. */
interface PostHogClient {
  init(key: string, config: Record<string, unknown>): void;
  register(properties: Record<string, unknown>): void;
}

export interface PostHogSurfaceOptions {
  /** Taxonomy: which app the event came from ("web-app", "ctrl", "pulse", …). */
  surface: string;
  /** Taxonomy: deployment environment, typically `import.meta.env.MODE`. */
  environment: string;
  /** Public project key. Empty/undefined → no-op, so keyless builds stay silent. */
  key: string | undefined;
  /** Ingestion host; defaults to the PostHog EU endpoint. */
  host?: string;
  /** Extra posthog.init config merged over `{ api_host }` (autocapture, session_recording, …). */
  init?: Record<string, unknown>;
}

/**
 * Initialize a PostHog surface with `{ surface, environment }` registered as
 * super-properties. Key-gated: returns false without initializing when no key
 * is configured. The caller owns the init-once guard.
 *
 * The key is the only gate this helper applies, and it is not the only gate a
 * surface needs. This call is what writes the year-long `ph_*` identifier — on
 * the registrable domain, shared with the landing and documentation sites,
 * which publish it as cookieless — so a surface behind a login calls it once a
 * session is confirmed and never on load, and a public one passes
 * `persistence: "memory"` (#1940).
 */
export function initPostHogSurface(client: PostHogClient, opts: PostHogSurfaceOptions): boolean {
  if (!opts.key) return false;
  client.init(opts.key, { api_host: opts.host ?? "https://eu.i.posthog.com", ...opts.init });
  client.register({ surface: opts.surface, environment: opts.environment });
  return true;
}
