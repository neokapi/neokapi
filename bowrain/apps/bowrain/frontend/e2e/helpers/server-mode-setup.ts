/**
 * Setup for Wails server mode (headless binary built with `-tags fts5,server`).
 *
 * In server mode the Go binary serves the frontend over HTTP and routes every
 * Wails binding call through the HTTP transport. No native window, no GUI deps
 * — but the same real backend the desktop app runs, talking to a real
 * bowrain-server. This is the transport the suite is migrating onto (#1579),
 * because `mock-backend.ts` models Wails bindings by a hand-maintained ID map
 * and most of the app now speaks REST through `ProxyRequest`, which is not in
 * it.
 *
 * Prerequisites — all three, or the app will not mount:
 *
 * 1. `make -C bowrain build-headless`. The Makefile passes `fts5,server` as one
 *    tag list; two `-tags` flags would silently drop fts5 and the binary would
 *    fail at runtime the first time a spec touched memory or terms.
 * 2. The e2e Docker stack up:
 *    `docker compose -f bowrain/e2e/server/compose.yaml up -d --wait`.
 * 3. The binary started with `BOWRAIN_SERVER_URL` and a **real** `BOWRAIN_TOKEN`
 *    from the device flow against that stack — see `authenticate()` in
 *    `./api-client`. `backend/connection.go` auto-connects from those two on the
 *    first `refresh()`, which is the supported headless path and keeps the auth
 *    flow real, as CLAUDE.md requires.
 *
 * There used to be a fourth option here — `window.__skipConnection` — and it no
 * longer works. `App.tsx` demotes `ready` back to `connecting` whenever the
 * connection state is neither connected nor offline, which is correct product
 * behaviour for desktop sign-out and which unconditionally defeats the flag.
 * Without a real session the app renders the sign-in screen. The escape hatch is
 * gone: a session is the only way in, which is the direction we wanted anyway.
 */
import type { Page } from "@playwright/test";

/**
 * Navigates to the headless binary's HTTP server and waits for the app shell.
 *
 * The gate is testids only. Keying a readiness gate on dashboard copy is what
 * kept this suite red for a fortnight after #1363 reworded the hero, so the
 * wait accepts either the empty-workspace onboarding or the nav rail — the two
 * shapes the shell takes once it has mounted.
 */
export async function setupServerModeApp(page: Page): Promise<void> {
  await page.goto("/");

  await page
    .getByTestId("empty-projects")
    .or(page.getByTestId("nav-translate"))
    .first()
    .waitFor({ state: "visible", timeout: 30000 });
}
