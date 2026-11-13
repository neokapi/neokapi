// How the app uses the system's browser session for sign-in and sign-out, kept
// apart from the session state so it can be tested without native modules.
//
// Switching accounts needs the identity provider to forget the last sign-in.
// Two mechanisms do that, one per platform:
//
// - iOS: sign-in runs in an ephemeral ASWebAuthenticationSession
//   (preferEphemeralSession). It shares no cookies with Safari and keeps none
//   after it closes, so every sign-in asks for an account and there is no
//   provider cookie left to clear at sign-out. It also skips the system
//   prompt that asks to share website data.
// - Android: Custom Tabs share the browser's cookie jar and have no ephemeral
//   mode, so sign-out opens the provider's end-session URL, which the server
//   returns from POST /auth/logout, and waits for the provider to redirect to
//   signedOutRedirectUri.
import type { AuthSessionOpenOptions, WebBrowserAuthSessionResult } from "expo-web-browser";

export type OpenAuthSession = (
  url: string,
  redirectUrl: string,
  options?: AuthSessionOpenOptions,
) => Promise<WebBrowserAuthSessionResult>;

/** The browser-session options for sign-in and sign-out on a platform. */
export function authSessionOptions(platform: string): AuthSessionOpenOptions {
  return platform === "ios" ? { preferEphemeralSession: true } : {};
}

/**
 * Whether sign-out has a provider session to end in the browser. Only when the
 * server returned an end-session URL and the platform's sign-in left a cookie.
 */
export function needsProviderSignOut(platform: string, endSessionUrl?: string): boolean {
  return Boolean(endSessionUrl) && platform !== "ios";
}

export type ProviderSignOut = "skipped" | "ended" | "cancelled" | "failed";

/**
 * End the identity provider's session after the server's sign-out. A failure
 * never stops the sign-out on the device; it is reported so the caller can
 * tell, and the device is signed out either way.
 */
export async function endProviderSession(opts: {
  platform: string;
  endSessionUrl?: string;
  redirectUri: string;
  open: OpenAuthSession;
}): Promise<ProviderSignOut> {
  if (!opts.endSessionUrl || !needsProviderSignOut(opts.platform, opts.endSessionUrl)) {
    return "skipped";
  }
  try {
    const res = await opts.open(
      opts.endSessionUrl,
      opts.redirectUri,
      authSessionOptions(opts.platform),
    );
    return res.type === "success" ? "ended" : "cancelled";
  } catch {
    return "failed";
  }
}
