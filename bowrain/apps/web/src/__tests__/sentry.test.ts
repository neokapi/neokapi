import { describe, it, expect, vi, beforeEach } from "vite-plus/test";

const init = vi.fn();
const setTag = vi.fn();

vi.mock("@sentry/react", () => ({
  init,
  setTag,
  browserTracingIntegration: () => ({ name: "BrowserTracing" }),
}));
vi.mock("posthog-js", () => ({ default: {} }));

describe("sentry integration", () => {
  beforeEach(() => {
    vi.resetModules();
    init.mockClear();
    setTag.mockClear();
  });

  it("stays silent without a DSN", async () => {
    vi.stubEnv("VITE_SENTRY_DSN", "");
    const { initSentry, sentryEnabled } = await import("../sentry");
    initSentry();
    expect(init).not.toHaveBeenCalled();
    expect(sentryEnabled()).toBe(false);
  });

  it("collects no user, cookie, header or body data", async () => {
    vi.stubEnv("VITE_SENTRY_DSN", "https://key@o0.ingest.sentry.io/0");
    const { initSentry, sentryEnabled } = await import("../sentry");
    initSentry();
    expect(init).toHaveBeenCalledTimes(1);
    const options = init.mock.calls[0][0];
    expect(options.dataCollection).toEqual({
      userInfo: false,
      cookies: false,
      httpHeaders: false,
      httpBodies: [],
    });
    expect(options.beforeSend).toBeTypeOf("function");
    expect(setTag).toHaveBeenCalledWith("surface", "web-app");
    expect(sentryEnabled()).toBe(true);
  });
});
