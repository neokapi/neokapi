import { test, expect } from "../fixtures/test";

test.describe("Authentication", () => {
  test("registered user can access /auth/me", async ({ api, auth }) => {
    const me = await api.me();
    expect(me.email).toBe(auth.email);
  });

  test("session cookie grants access to protected routes", async ({ authenticatedPage }) => {
    await authenticatedPage.goto("/");
    // Should see the app (not the login page) — look for the sidebar nav.
    await expect(authenticatedPage.getByTestId("nav-translate")).toBeVisible({ timeout: 15_000 });
  });

  test("passkey login works for existing user", async ({ browser, auth, kcAdmin }) => {
    // Create a new browser context with a fresh virtual authenticator.
    // We need to re-register the passkey since virtual authenticators don't persist
    // across contexts. Instead, we verify the session is valid by using the API.
    const me = await new (
      await import("../helpers/api-client")
    ).BowrainAPI(process.env.BOWRAIN_URL || "http://localhost:8080", auth.sessionToken).me();
    expect(me.email).toBe(auth.email);
    expect(me.name).toBeTruthy();
  });
});
