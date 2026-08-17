/* eslint-disable @typescript-eslint/unbound-method */
import { describe, it, expect, vi, beforeAll, beforeEach } from "vite-plus/test";

vi.mock("posthog-js", () => ({
  default: {
    init: vi.fn(),
    capture: vi.fn(),
    register: vi.fn(),
  },
}));

describe("pulse analytics", () => {
  // The first dynamic import in the file pays a one-time cold cost — the vite
  // transform of the module graph plus test-environment setup — that the log
  // attributes ~5s to on a cold cache. Charged against a single test's default
  // 5s budget it flakes the first case on slower CI runners, so we absorb it
  // here in a hook with generous headroom; every test then imports warm.
  beforeAll(async () => {
    await import("./analytics");
  }, 30000);

  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
    vi.clearAllMocks();
  });

  // Pulse dashboards are readable without a session, so there is no
  // authentication to start analytics behind. It carries the other posture the
  // parent domain publishes instead: memory-only persistence, so a visit writes
  // no identifier to the domain the landing and documentation sites share
  // (#1940).
  it("initializes cookieless, with Do Not Track honoured", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { initAnalytics } = await import("./analytics");
    initAnalytics();
    expect(vi.mocked(posthog.init)).toHaveBeenCalledWith("phc_test_key", {
      api_host: "https://eu.i.posthog.com",
      persistence: "memory",
      respect_dnt: true,
      disable_session_recording: true,
      disable_surveys: true,
      capture_pageview: false,
      capture_pageleave: true,
      autocapture: false,
    });
    expect(vi.mocked(posthog.register)).toHaveBeenCalledWith({
      surface: "pulse",
      environment: expect.any(String) as string,
    });
  });

  it("stays silent in a keyless build", async () => {
    const posthog = (await import("posthog-js")).default;
    const { initAnalytics, capture } = await import("./analytics");
    initAnalytics();
    capture("$pageview", { route: "/$workspace" });
    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.capture)).not.toHaveBeenCalled();
  });
});
