import { RestApiAdapter } from "@neokapi/ui";

export const api = new RestApiAdapter();

// When both access and refresh tokens are invalid, redirect to OIDC.
// Skip redirect for /claim/ and /join/ routes — those pages handle auth themselves.
api.onSessionExpired = () => {
  const path = window.location.pathname;
  if (path.startsWith("/claim/") || path.startsWith("/join/") || path.startsWith("/device/")) {
    return;
  }
  window.location.href = "/api/v1/auth/login";
};
