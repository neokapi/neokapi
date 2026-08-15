/**
 * The channel a visitor arrived through, carried as far as the signup event.
 *
 * The landing forwards its inbound `utm_*` parameters onto every app link and
 * stamps `utm_source=bowrain-landing` when there is none, so a visitor reaching
 * the app root carries the campaign in the query string. That query string is
 * gone by the time the account exists: the visitor bounces to the identity
 * provider and returns to a bare callback URL, and the account is created
 * server-side during that callback.
 *
 * So the label is stashed in a short-lived cookie — the mechanism
 * `stashIntendedPlan` already uses for the same round-trip. `SameSite=Lax` is
 * what makes it work: the callback is a top-level GET navigation from another
 * site, which is exactly the case Lax still sends cookies for. The server reads
 * it on that request and puts it on `user_signup`.
 *
 * The value is a LABEL, not free text. The server re-validates it — lowercase,
 * `[a-z0-9._-]`, 64 characters — and drops anything outside that shape rather
 * than repairing it, so a cookie written by hand cannot widen an analytics
 * property into a free-text field.
 */

const COOKIE = "bowrain_acquisition";

/** The shape the server accepts; anything else is dropped rather than cleaned. */
const LABEL = /^[a-z0-9._-]{1,64}$/;

/**
 * The acquisition label for the current visit: the campaign the visitor arrived
 * with, or the host that referred them, or null when there is nothing to say.
 * A referrer from this same origin says nothing — it is the app talking to
 * itself — so it is ignored.
 */
export function currentAcquisitionSource(): string | null {
  const campaign = new URLSearchParams(window.location.search).get("utm_source");
  const candidate = campaign ?? referringHost();
  if (!candidate) return null;
  const label = candidate.trim().toLowerCase();
  return LABEL.test(label) ? label : null;
}

function referringHost(): string | null {
  if (!document.referrer) return null;
  try {
    const referrer = new URL(document.referrer);
    return referrer.origin === window.location.origin ? null : referrer.hostname;
  } catch {
    return null;
  }
}

/**
 * Stash the visit's acquisition label so it survives the OIDC round-trip.
 * A visit that carries no label leaves an earlier one alone: a returning
 * visitor arriving directly has not changed where their account came from.
 * One hour, like the intended plan — long enough for a signup, short enough
 * that a stale label cannot attribute an account it had nothing to do with.
 */
export function stashAcquisitionSource(): void {
  const source = currentAcquisitionSource();
  if (!source) return;
  document.cookie = `${COOKIE}=${encodeURIComponent(source)}; path=/; max-age=3600; SameSite=Lax`;
}
