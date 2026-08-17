/* eslint-disable @typescript-eslint/unbound-method */
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";

vi.mock("posthog-js", () => ({
  default: {
    init: vi.fn(),
    capture: vi.fn(),
    register: vi.fn(),
  },
}));

describe("pulse analytics", () => {
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
