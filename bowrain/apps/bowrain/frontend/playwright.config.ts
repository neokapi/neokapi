import { defineConfig } from "@playwright/test";

const useServerMode = !!process.env.WAILS_SERVER_MODE;

/**
 * Ports are overridable so two checkouts can run the suite at once, and
 * `reuseExistingServer` is false on both branches so a busy port is a loud
 * failure rather than a silent adoption.
 *
 * It used to be `!process.env.CI`, which is harmless in CI — where the port is
 * always free — and therefore only ever bit locally, in runs nobody reads. Both
 * branches have since done it. A dev-mode run bound to another worktree's Vite
 * server on 5173 and produced a confident "the app renders blank", caught only
 * because the served module paths named a different worktree. A server-mode run
 * bound to an unrelated `bowrain-server` container that happened to publish
 * 8090, which redirected the app to that stack's Keycloak and reported the
 * suite as failing at the launch probe. Neither run started the process it was
 * configured to test, and neither said so.
 *
 * Playwright has no `strictPort`; refusing to reuse is the equivalent — it
 * errors with "port is already used" instead of testing a stranger's process.
 */
const DEV_PORT = Number(process.env.BOWRAIN_E2E_DEV_PORT ?? 5173);
const SERVER_PORT = Number(process.env.WAILS_SERVER_PORT ?? 8090);
const port = useServerMode ? SERVER_PORT : DEV_PORT;

/**
 * A throwaway config/data root for the headless binary. Without one it reads
 * the developer's real bowrain-desktop config — the server URL and stored
 * session they last signed in with — and the suite silently exercises that
 * instead of a known state. HOME is set too, not just the XDG vars: on macOS
 * `os.UserConfigDir()` ignores XDG entirely and resolves under HOME.
 */
const isolatedHome = process.env.BOWRAIN_E2E_HOME;
const isolationEnv: Record<string, string> = isolatedHome
  ? {
      HOME: isolatedHome,
      XDG_CONFIG_HOME: `${isolatedHome}/.config`,
      XDG_DATA_HOME: `${isolatedHome}/.local/share`,
      XDG_CACHE_HOME: `${isolatedHome}/.cache`,
    }
  : {};

export default defineConfig({
  testDir: "./e2e",
  testIgnore: ["recordings.spec.ts"],
  timeout: 30000,
  globalTimeout: process.env.CI ? 600000 : 0, // 10 min cap in CI (204 tests)
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "html",
  use: {
    baseURL: `http://localhost:${port}`,
    headless: true,
    screenshot: "only-on-failure",
    trace: "on-first-retry",
  },
  webServer: useServerMode
    ? {
        command: "cd ../../../.. && bin/bowrain-headless",
        port: SERVER_PORT,
        timeout: 30000,
        reuseExistingServer: false,
        env: { WAILS_SERVER_PORT: String(SERVER_PORT), ...isolationEnv },
      }
    : {
        command: `vp run dev -- --port ${DEV_PORT} --strictPort`,
        port: DEV_PORT,
        timeout: 30000,
        reuseExistingServer: false,
      },
});
