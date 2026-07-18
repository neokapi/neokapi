/* eslint-disable @typescript-eslint/unbound-method */
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";

// Mock posthog-js before importing our module.
vi.mock("posthog-js", () => ({
  default: {
    init: vi.fn(),
    identify: vi.fn(),
    reset: vi.fn(),
    capture: vi.fn(),
    group: vi.fn(),
    register: vi.fn(),
  },
}));

describe("posthog integration", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
  });

  it("does not initialize when VITE_POSTHOG_KEY is not set", async () => {
    const posthog = (await import("posthog-js")).default;
    const { initPostHog } = await import("../posthog");
    initPostHog();
    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.register)).not.toHaveBeenCalled();
  });

  it("initializes with router-driven pageviews when VITE_POSTHOG_KEY is set", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { initPostHog } = await import("../posthog");
    initPostHog();
    expect(vi.mocked(posthog.init)).toHaveBeenCalledWith("phc_test_key", {
      api_host: "https://eu.i.posthog.com",
      // SPA pageviews come from the router subscription, not full loads.
      capture_pageview: false,
      capture_pageleave: true,
      autocapture: true,
      // Session replay, privacy-first (all inputs masked).
      disable_session_recording: false,
      session_recording: {
        maskAllInputs: true,
        maskTextSelector: "[data-sensitive]",
      },
    });
  });

  it("registers the surface + environment super-properties on init", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { initPostHog } = await import("../posthog");
    initPostHog();
    expect(vi.mocked(posthog.register)).toHaveBeenCalledWith({
      surface: "web-app",
      environment: expect.any(String) as string,
    });
  });

  it("identifyUser is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { identifyUser } = await import("../posthog");
    identifyUser("user-1", { email: "test@example.com" });
    expect(vi.mocked(posthog.identify)).not.toHaveBeenCalled();
  });

  it("identifyUser calls posthog.identify with key", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser } = await import("../posthog");
    identifyUser("user-1", { email: "test@example.com" });
    expect(vi.mocked(posthog.identify)).toHaveBeenCalledWith("user-1", {
      email: "test@example.com",
    });
  });

  it("captureEvent is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { captureEvent } = await import("../posthog");
    captureEvent("feature_entered", { feature: "translate" });
    expect(vi.mocked(posthog.capture)).not.toHaveBeenCalled();
  });

  it("captureEvent forwards to posthog.capture with key", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { captureEvent } = await import("../posthog");
    captureEvent("$pageview", { path_pattern: "/$workspace/brand/concepts" });
    expect(vi.mocked(posthog.capture)).toHaveBeenCalledWith("$pageview", {
      path_pattern: "/$workspace/brand/concepts",
    });
  });

  it("groupIdentify is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { groupIdentify } = await import("../posthog");
    groupIdentify("workspace", "ws-1");
    expect(vi.mocked(posthog.group)).not.toHaveBeenCalled();
  });

  it("groupIdentify forwards to posthog.group with key", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { groupIdentify } = await import("../posthog");
    groupIdentify("workspace", "ws-1");
    expect(vi.mocked(posthog.group)).toHaveBeenCalledWith("workspace", "ws-1", undefined);
  });

  it("resetPostHog is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { resetPostHog } = await import("../posthog");
    resetPostHog();
    expect(vi.mocked(posthog.reset)).not.toHaveBeenCalled();
  });
});
