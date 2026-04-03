// ---------------------------------------------------------------------------
// OIDC auth against the bowrain-admin Keycloak realm
// Uses authorization code flow with PKCE (public client in browser).
// ---------------------------------------------------------------------------

function resolveIssuerUrl(): string {
  if (import.meta.env.VITE_ADMIN_OIDC_ISSUER_URL) {
    return import.meta.env.VITE_ADMIN_OIDC_ISSUER_URL;
  }
  // Derive from current hostname: ctrl[.dev].bowrain.cloud → auth[.dev].bowrain.cloud
  const host = window.location.hostname;
  if (host.startsWith("ctrl.")) {
    return `https://auth.${host.slice(5)}/realms/bowrain-admin`;
  }
  return "http://localhost:8180/realms/bowrain-admin";
}

const ISSUER_URL = resolveIssuerUrl();
const CLIENT_ID = import.meta.env.VITE_ADMIN_OIDC_CLIENT_ID ?? "bowrain-admin";
const REDIRECT_URI = `${window.location.origin}/auth/callback`;
const TOKEN_KEY = "bowrain_admin_token";
const REFRESH_KEY = "bowrain_admin_refresh";
const VERIFIER_KEY = "bowrain_admin_verifier";

interface TokenResponse {
  access_token: string;
  refresh_token?: string;
  expires_in: number;
  token_type: string;
}

interface TokenPayload {
  exp: number;
  email?: string;
  name?: string;
  preferred_username?: string;
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
// Token helpers
// ---------------------------------------------------------------------------

function parseToken(token: string): TokenPayload | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const payload = JSON.parse(atob(parts[1].replace(/-/g, "+").replace(/_/g, "/")));
    return payload as TokenPayload;
  } catch {
    return null;
  }
}

function isTokenExpired(token: string): boolean {
  const payload = parseToken(token);
  if (!payload) return true;
  // Consider expired 30 seconds before actual expiry
  return payload.exp * 1000 < Date.now() + 30_000;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

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

  window.location.href = `${ISSUER_URL}/protocol/openid-connect/auth?${params.toString()}`;
}

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

  const response = await fetch(`${ISSUER_URL}/protocol/openid-connect/token`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body,
  });

  if (!response.ok) {
    throw new Error(`Token exchange failed: ${response.status}`);
  }

  const data = (await response.json()) as TokenResponse;
  sessionStorage.setItem(TOKEN_KEY, data.access_token);
  if (data.refresh_token) {
    sessionStorage.setItem(REFRESH_KEY, data.refresh_token);
  }
}

export async function refreshToken(): Promise<boolean> {
  const refresh = sessionStorage.getItem(REFRESH_KEY);
  if (!refresh) return false;

  try {
    const body = new URLSearchParams({
      grant_type: "refresh_token",
      client_id: CLIENT_ID,
      refresh_token: refresh,
    });

    const response = await fetch(`${ISSUER_URL}/protocol/openid-connect/token`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
    });

    if (!response.ok) return false;

    const data = (await response.json()) as TokenResponse;
    sessionStorage.setItem(TOKEN_KEY, data.access_token);
    if (data.refresh_token) {
      sessionStorage.setItem(REFRESH_KEY, data.refresh_token);
    }
    return true;
  } catch {
    return false;
  }
}

export function getToken(): string | null {
  const token = sessionStorage.getItem(TOKEN_KEY);
  if (!token) return null;
  if (isTokenExpired(token)) return null;
  return token;
}

export function isAuthenticated(): boolean {
  return getToken() !== null;
}

export function getAdminUser(): { email: string; name: string } | null {
  const token = sessionStorage.getItem(TOKEN_KEY);
  if (!token) return null;
  const payload = parseToken(token);
  if (!payload) return null;
  return {
    email: payload.email ?? payload.preferred_username ?? "admin",
    name: payload.name ?? payload.email ?? "Admin",
  };
}

export function logout(): void {
  const token = sessionStorage.getItem(TOKEN_KEY);
  sessionStorage.removeItem(TOKEN_KEY);
  sessionStorage.removeItem(REFRESH_KEY);
  sessionStorage.removeItem(VERIFIER_KEY);

  // Redirect to Keycloak end session
  const params = new URLSearchParams({
    client_id: CLIENT_ID,
    post_logout_redirect_uri: window.location.origin,
  });
  if (token) {
    params.set("id_token_hint", token);
  }

  window.location.href = `${ISSUER_URL}/protocol/openid-connect/logout?${params.toString()}`;
}
