/* eslint-disable @typescript-eslint/unbound-method */
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";

vi.mock("posthog-js", () => ({
  default: {
    init: vi.fn(),
    capture: vi.fn(),
    register: vi.fn(),
  },
}));

describe("ctrl analytics", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
    vi.clearAllMocks();
  });

  // ctrl is served from a subdomain of the domain the landing and documentation
  // sites publish as cookieless. `posthog.init` writes a year-long identifier
  // scoped to that shared parent, so an anonymous visit — which ctrl answers by
  // bouncing to the admin identity provider — must not reach it (#1940).
  it("initializes nothing on import, even with a key configured", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    await import("./analytics");
    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.register)).not.toHaveBeenCalled();
  });

  it("captures nothing before an admin session is confirmed", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { capture, capturePageview } = await import("./analytics");
    capture("admin_action", { kind: "override" });
    capturePageview("/workspaces");
    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.capture)).not.toHaveBeenCalled();
  });

  it("starts on a confirmed session, with pageviews left to the router", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { startAnalytics, capturePageview } = await import("./analytics");
    startAnalytics();
    startAnalytics();
    expect(vi.mocked(posthog.init)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(posthog.init)).toHaveBeenCalledWith("phc_test_key", {
      api_host: "https://eu.i.posthog.com",
      capture_pageview: false,
      capture_pageleave: true,
      autocapture: false,
    });
    expect(vi.mocked(posthog.register)).toHaveBeenCalledWith({
      surface: "ctrl",
      environment: expect.any(String) as string,
    });
    capturePageview("/workspaces");
    expect(vi.mocked(posthog.capture)).toHaveBeenCalledWith("$pageview", {
      route: "/workspaces",
    });
  });

  it("stays silent in a keyless build even after a session", async () => {
    const posthog = (await import("posthog-js")).default;
    const { startAnalytics, capture } = await import("./analytics");
    startAnalytics();
    capture("admin_action");
    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.capture)).not.toHaveBeenCalled();
  });
});
