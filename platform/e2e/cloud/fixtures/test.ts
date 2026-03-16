/**
 * Playwright test fixture that provides:
 * - Virtual WebAuthn authenticator (via CDP)
 * - Fresh user registration through Keycloak (email + passkey)
 * - Authenticated BowrainAPI client
 * - Cleanup of test user after tests
 *
 * Environment variables:
 *   BOWRAIN_URL             — Server URL (default: http://localhost:8080)
 *   KEYCLOAK_URL            — Keycloak URL (default: derived from BOWRAIN_URL)
 *   KEYCLOAK_ADMIN_PASSWORD — Keycloak admin password (required)
 *   E2E_USER_EMAIL          — Override test user email (default: auto-generated)
 */

import { test as base, expect, type CDPSession, type Page } from "@playwright/test";
import { KeycloakAdmin } from "../helpers/keycloak-admin";
import { BowrainAPI, waitForReady, type ReadinessInfo } from "../helpers/api-client";

// ── Configuration ──────────────────────────────────────────────────────

const BOWRAIN_URL = process.env.BOWRAIN_URL || "http://localhost:8080";

/** Derive Keycloak URL from the server URL convention. */
function defaultKeycloakUrl(): string {
  if (process.env.KEYCLOAK_URL) return process.env.KEYCLOAK_URL;
  const url = new URL(BOWRAIN_URL);
  if (url.hostname === "localhost") {
    return `http://localhost:8180`;
  }
  // Cloud convention: auth.dev.bowrain.cloud for dev.bowrain.cloud
  return `https://auth.${url.hostname.replace(/^www\./, "")}`;
}

const KEYCLOAK_URL = defaultKeycloakUrl();
const KEYCLOAK_ADMIN_PASSWORD = process.env.KEYCLOAK_ADMIN_PASSWORD || "admin";

function testUserEmail(): string {
  if (process.env.E2E_USER_EMAIL) return process.env.E2E_USER_EMAIL;
  const id = Math.random().toString(36).slice(2, 8);
  return `e2e-${id}@test.bowrain.cloud`;
}

// ── Fixtures ───────────────────────────────────────────────────────────

export interface AuthContext {
  email: string;
  name: string;
  /** Session cookie value extracted after OIDC login. */
  sessionToken: string;
}

export interface TestFixtures {
  kcAdmin: KeycloakAdmin;
  readiness: ReadinessInfo;
  api: BowrainAPI;
  auth: AuthContext;
  authenticatedPage: Page;
}

export const test = base.extend<TestFixtures>({
  kcAdmin: async ({}, use) => {
    const admin = new KeycloakAdmin({
      baseUrl: KEYCLOAK_URL,
      adminPassword: KEYCLOAK_ADMIN_PASSWORD,
    });
    await use(admin);
  },

  readiness: async ({}, use) => {
    const info = await waitForReady(BOWRAIN_URL);
    await use(info);
  },

  auth: [
    async ({ browser, kcAdmin }, use) => {
      const email = testUserEmail();
      const name = "E2E Test User";

      // Clean up any leftover user from a previous run.
      await kcAdmin.deleteUserByEmail(email);

      // Create a fresh browser context with a virtual WebAuthn authenticator.
      const context = await browser.newContext({
        baseURL: BOWRAIN_URL,
        viewport: { width: 1280, height: 800 },
      });
      const page = await context.newPage();
      const cdp = await context.newCDPSession(page);
      await enableVirtualAuthenticator(cdp);

      // ── Step 1: Navigate to app → SSO → Keycloak ──────────────

      await page.goto("/");
      await expect(page.getByRole("button", { name: /sign in/i })).toBeVisible({
        timeout: 15_000,
      });
      await page.getByRole("button", { name: /sign in/i }).click();

      // Now on Keycloak login page. Click "Register".
      await expect(page.getByRole("link", { name: /register/i })).toBeVisible({
        timeout: 15_000,
      });
      await page.getByRole("link", { name: /register/i }).click();

      // ── Step 2: Fill registration form ─────────────────────────

      await expect(page.locator("#email")).toBeVisible({ timeout: 10_000 });
      await page.locator("#email").fill(email);

      // Handle first/last name fields if present.
      if (await page.locator("#firstName").isVisible().catch(() => false)) {
        await page.locator("#firstName").fill("E2E");
      }
      if (await page.locator("#lastName").isVisible().catch(() => false)) {
        await page.locator("#lastName").fill("Test User");
      }

      await page.getByRole("button", { name: /register/i }).click();

      // ── Step 3: Handle email verification ──────────────────────
      // Keycloak requires email verification. We use the admin API
      // to mark it verified, keeping the passkey enrollment action.

      // Wait a moment for Keycloak to create the user.
      await page.waitForTimeout(1_000);

      const user = await kcAdmin.findUser(email);
      if (user) {
        await kcAdmin.verifyEmail(user.id);
      }

      // Check if we landed on the verify-email page.
      const onVerifyPage = await page
        .getByText(/verify.*email|check.*inbox/i)
        .isVisible({ timeout: 5_000 })
        .catch(() => false);

      if (onVerifyPage) {
        // Re-initiate the OIDC flow — Keycloak will pick up the existing
        // session and proceed to the next required action (passkey enrollment).
        await page.goto(`${BOWRAIN_URL}/api/v1/auth/login`);
      }

      // ── Step 4: Passkey enrollment ─────────────────────────────
      // Keycloak shows the WebAuthn register page. The useScript hook
      // auto-triggers navigator.credentials.create() which the virtual
      // authenticator handles. If there's a "Register" button, click it.

      // Wait for either:
      // a) The passkey registration page (button id="authenticateWebAuthnButton")
      // b) Direct redirect to the app (if passkey enrollment was skipped)
      const passkeyButton = page.locator("#authenticateWebAuthnButton");
      const inApp = page.locator("[data-testid='nav-translate']").or(
        page.getByText(/workspace|create.*workspace/i),
      );

      await expect(passkeyButton.or(inApp)).toBeVisible({ timeout: 20_000 });

      if (await passkeyButton.isVisible().catch(() => false)) {
        // The useScript hook should auto-trigger the WebAuthn ceremony.
        // If it hasn't fired yet, click the button to trigger it.
        await passkeyButton.click().catch(() => {
          // Already triggered by useScript — that's fine.
        });

        // Wait for Keycloak to process the registration and redirect to app.
        await page.waitForURL(
          (url) => !url.href.includes("/realms/"),
          { timeout: 30_000 },
        );
      }

      // ── Step 5: Extract session token ──────────────────────────

      // Wait for the app to fully load (ensures the callback was processed).
      await page.waitForLoadState("networkidle");

      const cookies = await context.cookies(BOWRAIN_URL);
      const sessionCookie = cookies.find((c) => c.name === "bowrain_session");
      if (!sessionCookie) {
        throw new Error(
          `No bowrain_session cookie after login.\n` +
            `URL: ${page.url()}\n` +
            `Cookies: ${cookies.map((c) => c.name).join(", ")}`,
        );
      }

      const auth: AuthContext = {
        email,
        name,
        sessionToken: sessionCookie.value,
      };

      await context.close();
      await use(auth);

      // ── Cleanup ────────────────────────────────────────────────
      await kcAdmin.deleteUserByEmail(email);
    },
    { scope: "worker" },
  ],

  api: async ({ auth }, use) => {
    const api = new BowrainAPI(BOWRAIN_URL, auth.sessionToken);
    await use(api);
  },

  authenticatedPage: async ({ browser, auth }, use) => {
    const context = await browser.newContext({
      baseURL: BOWRAIN_URL,
      viewport: { width: 1280, height: 800 },
    });

    const url = new URL(BOWRAIN_URL);
    await context.addCookies([
      {
        name: "bowrain_session",
        value: auth.sessionToken,
        domain: url.hostname,
        path: "/api/",
        httpOnly: true,
        sameSite: "Lax",
        secure: url.protocol === "https:",
      },
    ]);

    const page = await context.newPage();
    await use(page);
    await context.close();
  },
});

export { expect };

// ── Virtual WebAuthn Authenticator ─────────────────────────────────────

async function enableVirtualAuthenticator(cdp: CDPSession): Promise<string> {
  await cdp.send("WebAuthn.enable" as any, { enableUI: false });

  const result = await cdp.send("WebAuthn.addVirtualAuthenticator" as any, {
    options: {
      protocol: "ctap2",
      transport: "internal",
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  });

  return (result as any).authenticatorId;
}
