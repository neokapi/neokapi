/* eslint-disable @typescript-eslint/unbound-method */
import { describe, it, expect, vi, beforeEach } from "vite-plus/test";

// Mock posthog-js before importing our module.
vi.mock("posthog-js", () => ({
  default: {
    init: vi.fn(),
    identify: vi.fn(),
    reset: vi.fn(),
    capture: vi.fn(),
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
  });

  it("initializes when VITE_POSTHOG_KEY is set", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { initPostHog } = await import("../posthog");
    initPostHog();
    expect(vi.mocked(posthog.init)).toHaveBeenCalledWith("phc_test_key", {
      api_host: "https://eu.i.posthog.com",
      capture_pageview: true,
      capture_pageleave: true,
      autocapture: true,
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

  it("resetPostHog is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { resetPostHog } = await import("../posthog");
    resetPostHog();
    expect(vi.mocked(posthog.reset)).not.toHaveBeenCalled();
  });
});
