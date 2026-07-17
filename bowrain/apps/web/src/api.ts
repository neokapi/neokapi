import { RestApiAdapter } from "@neokapi/ui";

export const api = new RestApiAdapter();

// When both access and refresh tokens are invalid, redirect to OIDC.
// Skip redirect for /claim/, /join/, /device/, /github/setup — those pages
// handle auth themselves (and preserve their context across the round-trip).
api.onSessionExpired = () => {
  const path = window.location.pathname;
  if (
    path.startsWith("/claim/") ||
    path.startsWith("/join/") ||
    path.startsWith("/device/") ||
    path.startsWith("/github/setup")
  ) {
    return;
  }
  window.location.href = "/api/v1/auth/login";
};
