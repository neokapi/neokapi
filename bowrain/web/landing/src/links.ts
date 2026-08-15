// Shared outbound links. The app lives at app.bowrain.cloud (DECISIONS L6
// splittable URLs); contact is on the bowrain.cloud domain we control —
// never the unverified .com mailbox (epic 011).
export const APP_URL = "https://app.bowrain.cloud";
export const SIGNUP_URL = `${APP_URL}/`;
export const CONTACT_EMAIL = "hello@bowrain.cloud";

/**
 * Signup URL carrying the intended plan so the app can pre-select it after the
 * OIDC round-trip (the app reads `?plan`/`?seats` at its entry route). Free needs
 * no param — it is the default landing target — so callers pass a plan only for a
 * self-serve paid tier. The server re-validates the plan on checkout; the param
 * is only a hint, never a price.
 */
export function signupUrl(opts?: { plan?: string; seats?: number }): string {
  if (!opts?.plan) return SIGNUP_URL;
  const params = new URLSearchParams({ plan: opts.plan });
  if (opts.seats && opts.seats > 0) params.set("seats", String(opts.seats));
  return `${SIGNUP_URL}?${params.toString()}`;
}
// Display-only identity probe on the app API. The landing (bowrain.cloud) is a
// DIFFERENT origin from the app (app.bowrain.cloud) but same-site, so a
// credentialed fetch sends the SameSite=Lax session cookie; the server answers
// with only display fields (never a token). Used to render the signed-in CTA.
export const whoamiUrl = () => `${APP_URL}/api/v1/auth/whoami`;

export const GITHUB_URL = "https://github.com/neokapi";
// The kapi site is served from the Pages root; anything under /web/ is a build
// path, not a published one.
export const KAPI_SITE_URL = "https://neokapi.github.io/";

// Relative to the deploy base: /web/bowrain/docs/ on GitHub Pages today,
// /docs/ once bowrain.cloud serves the site.
export const docsUrl = () => `${import.meta.env.BASE_URL}docs/`;
