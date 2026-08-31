// ---------------------------------------------------------------------------
// Admin auth (BFF cookie session).
//
// The ctrl SPA runs the bowrain-admin OIDC flow (authorization code + PKCE,
// public client) in the browser against whichever provider fronts the admin
// pool — Keycloak (self-host) or Cognito (hosted prod) — but it does NOT keep
// the provider tokens. Instead it hands the resulting id_token to the server's
// /api/admin/auth/exchange endpoint, which verifies it and sets an HttpOnly
// admin session cookie. All admin API calls then authenticate with that cookie
// plus an X-Bowrain-Csrf header — no admin token lives in the browser.
// ---------------------------------------------------------------------------

function resolveIssuerUrl(): string {
  if (import.meta.env.VITE_ADMIN_OIDC_ISSUER_URL) {
    return import.meta.env.VITE_ADMIN_OIDC_ISSUER_URL;
  }
  // Derive from current hostname: ctrl[.dev].bowrain.cloud → auth[.dev].bowrain.cloud.
  // This is the Keycloak self-host default; hosted prod sets
  // VITE_ADMIN_OIDC_ISSUER_URL to the Cognito admin-pool issuer.
  const host = window.location.hostname;
  if (host.startsWith("ctrl.")) {
    return `https://auth.${host.slice(5)}/realms/bowrain-admin`;
  }
  return "http://localhost:8180/realms/bowrain-admin";
}

const ISSUER_URL = resolveIssuerUrl();

// Whether the admin pool is fronted by Cognito. Detected from the STABLE OIDC
// issuer host (cognito-idp.<region>.amazonaws.com), NOT the Managed Login domain:
// the latter may be a custom domain (auth.ctrl.bowrain.cloud) whose hostname is
// no longer an *.amazoncognito.com signal, so keying logout off it would wrongly
// treat Cognito as Keycloak and break sign-out.
const IS_COGNITO = (() => {
  try {
    return /^cognito-idp\.[a-z0-9-]+\.amazonaws\.com$/i.test(new URL(ISSUER_URL).hostname);
  } catch {
    return false;
  }
})();

const CLIENT_ID = import.meta.env.VITE_ADMIN_OIDC_CLIENT_ID ?? "bowrain-admin";
const REDIRECT_URI = `${window.location.origin}/auth/callback`;
const VERIFIER_KEY = "bowrain_admin_verifier";

// CSRF header the server requires on cookie-authenticated admin requests.
const CSRF_HEADER = "X-Bowrain-Csrf";

// Provider-neutral endpoint resolution. Keycloak and Cognito expose different
// OIDC endpoint paths (…/protocol/openid-connect/* vs the Managed Login
// domain's …/oauth2/*), so the flow reads them from the issuer's discovery
// document rather than hardcoding one provider's shape. Fetched once and cached
// for the page lifetime.
interface OidcEndpoints {
  authorization_endpoint: string;
  token_endpoint: string;
  end_session_endpoint?: string;
}

let endpointsPromise: Promise<OidcEndpoints> | null = null;

function discoverEndpoints(): Promise<OidcEndpoints> {
  if (!endpointsPromise) {
    endpointsPromise = fetch(`${ISSUER_URL}/.well-known/openid-configuration`).then((r) => {
      if (!r.ok) {
        throw new Error(`OIDC discovery failed: ${r.status}`);
      }
      return r.json() as Promise<OidcEndpoints>;
    });
  }
  return endpointsPromise;
}

/** React Query key for the current admin session probe (/api/admin/auth/me). */
export const ADMIN_SESSION_QUERY_KEY = ["admin-session"] as const;

export interface AdminUser {
  email: string;
  name: string;
}

interface TokenResponse {
  access_token: string;
  id_token?: string;
  refresh_token?: string;
  expires_in: number;
  token_type: string;
}

// Same-origin admin API base: every deployment fronts ctrl with a proxy that
// forwards /api/* to bowrain-server. Kept in sync with api.ts.
function adminApiBase(): string {
  return import.meta.env.VITE_ADMIN_API_URL ?? "/api/admin";
}

// ---------------------------------------------------------------------------
// PKCE helpers
// ---------------------------------------------------------------------------

function generateRandomString(length: number): string {
  const array = new Uint8Array(length);
  crypto.getRandomValues(array);
  return Array.from(array, (b) => b.toString(16).padStart(2, "0")).join("");
}

async function sha256(plain: string): Promise<ArrayBuffer> {
  const encoder = new TextEncoder();
  return crypto.subtle.digest("SHA-256", encoder.encode(plain));
}

function base64UrlEncode(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (const b of bytes) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

async function generatePKCE(): Promise<{ verifier: string; challenge: string }> {
  const verifier = generateRandomString(64);
  const hashed = await sha256(verifier);
  const challenge = base64UrlEncode(hashed);
  return { verifier, challenge };
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/** Start the admin OIDC flow by redirecting to the provider's authorize endpoint. */
export async function login(): Promise<void> {
  const { verifier, challenge } = await generatePKCE();
  sessionStorage.setItem(VERIFIER_KEY, verifier);

  const params = new URLSearchParams({
    response_type: "code",
    client_id: CLIENT_ID,
    redirect_uri: REDIRECT_URI,
    scope: "openid email profile",
    code_challenge: challenge,
    code_challenge_method: "S256",
  });

  const { authorization_endpoint } = await discoverEndpoints();
  window.location.href = `${authorization_endpoint}?${params.toString()}`;
}

/**
 * Complete the OIDC redirect: exchange the code for tokens at the provider, then
 * hand the id_token to the server, which verifies it and sets the HttpOnly
 * admin session cookie. The provider tokens are never persisted in the browser.
 */
export async function handleCallback(code: string): Promise<void> {
  const verifier = sessionStorage.getItem(VERIFIER_KEY);
  if (!verifier) {
    throw new Error("Missing PKCE verifier — please restart the login flow.");
  }
  sessionStorage.removeItem(VERIFIER_KEY);

  const body = new URLSearchParams({
    grant_type: "authorization_code",
    client_id: CLIENT_ID,
    code,
    redirect_uri: REDIRECT_URI,
    code_verifier: verifier,
  });

  const { token_endpoint } = await discoverEndpoints();
  const response = await fetch(token_endpoint, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });

  if (!response.ok) {
    throw new Error(`Token exchange failed: ${response.status}`);
  }

  const data = (await response.json()) as TokenResponse;
  if (!data.id_token) {
    throw new Error("No id_token in OIDC response");
  }

  // BFF boundary: the server verifies the id_token and sets the admin session
  // cookie. Nothing is stored client-side.
  const exchange = await fetch(`${adminApiBase()}/auth/exchange`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", [CSRF_HEADER]: "1" },
    body: JSON.stringify({ id_token: data.id_token }),
  });
  if (!exchange.ok) {
    throw new Error(`Admin session exchange failed: ${exchange.status}`);
  }
}

/**
 * Probe the current admin session. Returns the admin identity when the session
 * cookie is valid, or null (unauthenticated). Used as the React Query source of
 * truth for auth state.
 */
export async function fetchAdminSession(): Promise<AdminUser | null> {
  try {
    const resp = await fetch(`${adminApiBase()}/auth/me`, { credentials: "same-origin" });
    if (!resp.ok) return null;
    return (await resp.json()) as AdminUser;
  } catch {
    return null;
  }
}

/**
 * Clear the server admin session, then end the provider SSO session via the
 * provider's end_session_endpoint (both Keycloak and Cognito Managed Login
 * advertise one). Cognito's /logout is not OIDC-RP-initiated-logout compatible:
 * it wants client_id + logout_uri (a registered sign-out URL), whereas Keycloak
 * uses the standard post_logout_redirect_uri — so branch on the provider.
 */
export async function logout(): Promise<void> {
  try {
    await fetch(`${adminApiBase()}/auth/logout`, {
      method: "POST",
      credentials: "same-origin",
      headers: { [CSRF_HEADER]: "1" },
    });
  } catch {
    // Best-effort — still try to terminate the SSO session below.
  }

  const { end_session_endpoint } = await discoverEndpoints().catch((): OidcEndpoints => ({
    authorization_endpoint: "",
    token_endpoint: "",
  }));
  if (!end_session_endpoint) {
    // No RP-initiated logout advertised: the server session is already cleared;
    // just return to the app root.
    window.location.href = window.location.origin;
    return;
  }

  const params = new URLSearchParams({ client_id: CLIENT_ID });
  if (IS_COGNITO) {
    // Cognito: logout_uri must exactly match a registered sign-out URL.
    params.set("logout_uri", window.location.origin);
  } else {
    params.set("post_logout_redirect_uri", window.location.origin);
  }
  window.location.href = `${end_session_endpoint}?${params.toString()}`;
}
