/* eslint-disable @typescript-eslint/unbound-method */
import { describe, it, expect, vi, beforeEach, beforeAll } from "vite-plus/test";

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

/** The init config the web app is expected to start with. */
const WEB_APP_INIT = {
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
  // Core Web Vitals ($web_vitals).
  capture_performance: { web_vitals: true },
  // Real PostHog UI host (api_host may be the first-party proxy).
  ui_host: "https://eu.posthog.com",
  // The identifier stays on this host rather than the registrable domain the
  // cookieless landing and documentation sites share.
  cross_subdomain_cookie: false,
};

describe("posthog integration", () => {
  // The first dynamic import in the file pays a one-time cold cost — the vite
  // transform of the module graph plus test-environment setup — that the log
  // attributes ~5s to on a cold cache. Charged against a single test's default
  // 5s budget it flakes the first case on slower CI runners, so we absorb it
  // here in a hook with generous headroom; every test then imports warm.
  beforeAll(async () => {
    await import("../posthog");
  }, 30000);

  beforeEach(() => {
    vi.resetModules();
    vi.unstubAllEnvs();
    vi.clearAllMocks();
  });

  // The guarantee an anonymous visit rests on: loading the module — which is
  // what the app entry does — starts nothing. `posthog.init` is the call that
  // writes the year-long identifier on `.bowrain.cloud`, arms autocapture and
  // starts session replay, so a visitor who has not signed in leaves the origin
  // exactly as they found it.
  it("initializes nothing on import, even with a key configured", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    await import("../posthog");
    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.register)).not.toHaveBeenCalled();
  });

  it("exposes no way to start analytics without a user", async () => {
    const module = await import("../posthog");
    expect(Object.keys(module).sort()).toEqual([
      "captureEvent",
      "groupIdentify",
      "identifyUser",
      "posthog",
      "resetPostHog",
    ]);
  });

  it("captures nothing before a session, so the pre-auth leg sends no request", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { captureEvent, groupIdentify, resetPostHog } = await import("../posthog");

    captureEvent("$pageview", { path_pattern: "/" });
    captureEvent("signup_redirect_started", { has_plan: true }, { transport: "beacon" });
    groupIdentify("workspace", "ws-1");
    resetPostHog();

    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.capture)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.group)).not.toHaveBeenCalled();
    expect(vi.mocked(posthog.reset)).not.toHaveBeenCalled();
  });

  it("starts on the identified user, with router-driven pageviews", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser } = await import("../posthog");
    identifyUser("user-1", { email: "test@example.com" });
    expect(vi.mocked(posthog.init)).toHaveBeenCalledWith("phc_test_key", WEB_APP_INIT);
  });

  it("keeps the identifier on this host, off the domain the cookieless sites share", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser } = await import("../posthog");
    identifyUser("user-1");
    const [, config] = vi.mocked(posthog.init).mock.calls[0] as [string, Record<string, unknown>];
    // PostHog defaults this to true, which puts the cookie on `.bowrain.cloud` —
    // the domain the landing and documentation sites publish as cookieless.
    expect(config.cross_subdomain_cookie).toBe(false);
  });

  it("registers the surface + environment super-properties on init", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser } = await import("../posthog");
    identifyUser("user-1");
    expect(vi.mocked(posthog.register)).toHaveBeenCalledWith({
      surface: "web-app",
      environment: expect.any(String) as string,
    });
  });

  it("identifyUser is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { identifyUser } = await import("../posthog");
    identifyUser("user-1", { email: "test@example.com" });
    expect(vi.mocked(posthog.init)).not.toHaveBeenCalled();
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

  // Route loads re-run, and both the index and the workspace entry announce the
  // user; PostHog only needs telling once.
  it("identifies a repeated user once", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser } = await import("../posthog");
    identifyUser("user-1");
    identifyUser("user-1");
    identifyUser("user-2");
    expect(vi.mocked(posthog.init)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(posthog.identify)).toHaveBeenCalledTimes(2);
  });

  it("captureEvent is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { captureEvent } = await import("../posthog");
    captureEvent("feature_entered", { feature: "translate" });
    expect(vi.mocked(posthog.capture)).not.toHaveBeenCalled();
  });

  it("captureEvent forwards to posthog.capture once a session has started", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser, captureEvent } = await import("../posthog");
    identifyUser("user-1");
    captureEvent("$pageview", { path_pattern: "/$workspace/context/concepts" });
    expect(vi.mocked(posthog.capture)).toHaveBeenCalledWith("$pageview", {
      path_pattern: "/$workspace/context/concepts",
    });
  });

  it("captureEvent honours the beacon transport for events fired on the way out", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser, captureEvent } = await import("../posthog");
    identifyUser("user-1");
    captureEvent("checkout_started", { plan: "pro" }, { transport: "beacon" });
    expect(vi.mocked(posthog.capture)).toHaveBeenCalledWith(
      "checkout_started",
      { plan: "pro" },
      { transport: "sendBeacon" },
    );
  });

  it("groupIdentify is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { groupIdentify } = await import("../posthog");
    groupIdentify("workspace", "ws-1");
    expect(vi.mocked(posthog.group)).not.toHaveBeenCalled();
  });

  it("groupIdentify forwards to posthog.group once a session has started", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser, groupIdentify } = await import("../posthog");
    identifyUser("user-1");
    groupIdentify("workspace", "ws-1");
    expect(vi.mocked(posthog.group)).toHaveBeenCalledWith("workspace", "ws-1", undefined);
  });

  it("resetPostHog is a no-op without key", async () => {
    const posthog = (await import("posthog-js")).default;
    const { resetPostHog } = await import("../posthog");
    resetPostHog();
    expect(vi.mocked(posthog.reset)).not.toHaveBeenCalled();
  });

  // Sign-out resets, and the same user signing back in must be re-identified
  // rather than deduped away against the id PostHog no longer holds.
  it("re-identifies the same user after a reset", async () => {
    vi.stubEnv("VITE_POSTHOG_KEY", "phc_test_key");
    const posthog = (await import("posthog-js")).default;
    const { identifyUser, resetPostHog } = await import("../posthog");
    identifyUser("user-1");
    resetPostHog();
    identifyUser("user-1");
    expect(vi.mocked(posthog.reset)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(posthog.identify)).toHaveBeenCalledTimes(2);
  });
});
