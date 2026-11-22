// The sign-in redirect, read safely. The server sends the browser back to the
// app with a one-time code and the state the app generated; a token in the
// URL is never read, because another app registered for the same scheme could
// receive that URL. The code is useless without the PKCE verifier, which never
// leaves the app.

export type CallbackResult =
  | { ok: true; code: string }
  | { ok: false; reason: "state_mismatch" | "missing_code" | "error"; message?: string };

/** The query parameters of a redirect URL, with no URL parser of the platform's. */
function params(url: string): URLSearchParams {
  const i = url.indexOf("?");
  const q = i >= 0 ? url.slice(i + 1) : "";
  return new URLSearchParams(q.split("#")[0]);
}

/**
 * Read the sign-in redirect. A missing or different `state` is refused before
 * anything else is read, so a redirect the app did not start never signs it in.
 */
export function readCallback(url: string, expectedState: string): CallbackResult {
  const p = params(url);
  const state = p.get("state");
  if (!expectedState || state === null || !constantTimeEqual(state, expectedState)) {
    return { ok: false, reason: "state_mismatch" };
  }
  const error = p.get("error");
  if (error) return { ok: false, reason: "error", message: p.get("error_description") ?? error };
  const code = p.get("code");
  if (!code) return { ok: false, reason: "missing_code" };
  return { ok: true, code };
}

function constantTimeEqual(a: string, b: string): boolean {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}
