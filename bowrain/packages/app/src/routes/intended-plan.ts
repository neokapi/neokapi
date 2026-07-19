/**
 * Plan passthrough from the landing page. A visitor who clicks "Start free
 * trial" on a paid tier arrives at the app root with `?plan=<id>` (Team may add
 * `&seats=`), and the app pre-selects that plan on the billing page after signup.
 *
 * The plan must survive the OIDC round-trip: a brand-new visitor hits the app
 * unauthenticated, bounces to the identity provider, and returns to `/` with the
 * query string gone. We stash it in a short-lived cookie before the redirect —
 * the same mechanism `JoinPage` uses to return the user to `/join/:code` after
 * OIDC — and restore it once they land back: an already-onboarded user is caught
 * by the index route; a brand-new one by the welcome/onboarding step.
 *
 * The plan is only a HINT. The server re-validates it on checkout
 * (`HandleCreateCheckout` → `billing.SelfServePlans`) and resolves the price, so
 * the client trusts nothing here beyond "is this one of the self-serve ids".
 */

// The self-serve plans a landing CTA may pre-select. Mirrors
// billing.SelfServePlans (Pro, Team) — the server is authoritative and rejects
// anything else, so this is only a client-side allowlist that ignores junk.
export const SELF_SERVE_PLANS = ["pro", "team"] as const;

export type IntendedPlan = (typeof SELF_SERVE_PLANS)[number];

export function isIntendedPlan(value: unknown): value is IntendedPlan {
  return typeof value === "string" && (SELF_SERVE_PLANS as readonly string[]).includes(value);
}

/** validateSearch coercer: keep a valid self-serve plan id, drop anything else. */
export function searchPlan(value: unknown): IntendedPlan | undefined {
  return isIntendedPlan(value) ? value : undefined;
}

/**
 * validateSearch coercer for an optional positive seat count. The router
 * JSON-parses search values, so a numeric `?seats=3` arrives as a number and a
 * hand-typed one as a string — accept both, reject anything non-positive.
 */
export function searchSeats(value: unknown): number | undefined {
  const n = typeof value === "number" ? value : typeof value === "string" ? Number(value) : NaN;
  return Number.isInteger(n) && n > 0 ? n : undefined;
}

export interface IntendedPlanSelection {
  plan: IntendedPlan;
  seats?: number;
}

const COOKIE = "bowrain_intended_plan";

/**
 * Stash the intended plan before an auth redirect so it survives OIDC +
 * onboarding. Short-lived (1h) and SameSite=Lax so it rides the top-level GET
 * back from the identity provider but expires quickly if the user never returns.
 */
export function stashIntendedPlan(sel: IntendedPlanSelection): void {
  const parts = [`plan=${sel.plan}`];
  if (sel.seats && sel.seats > 0) parts.push(`seats=${sel.seats}`);
  document.cookie = `${COOKIE}=${encodeURIComponent(parts.join("&"))}; path=/; max-age=3600; SameSite=Lax`;
}

/** Read + validate the stashed intended plan, or null if none/invalid. */
export function readIntendedPlan(): IntendedPlanSelection | null {
  const raw = document.cookie
    .split("; ")
    .find((c) => c.startsWith(`${COOKIE}=`))
    ?.slice(COOKIE.length + 1);
  if (!raw) return null;
  const params = new URLSearchParams(decodeURIComponent(raw));
  const plan = params.get("plan");
  if (!isIntendedPlan(plan)) return null;
  return { plan, seats: searchSeats(params.get("seats") ?? undefined) };
}

/** Clear the stash once consumed (or when routing to billing). */
export function clearIntendedPlan(): void {
  document.cookie = `${COOKIE}=; path=/; max-age=0; SameSite=Lax`;
}
