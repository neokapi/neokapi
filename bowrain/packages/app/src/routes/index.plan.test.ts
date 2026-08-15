/**
 * Route-level cover for the plan passthrough. The cookie helper has its own
 * unit test (`intended-plan.test.ts`), and it passed while the index route
 * never reached the call — so everything here drives `indexRoute.beforeLoad`
 * with a real query client and an adapter that behaves like the server does for
 * the visitor the feature exists for: one with no session.
 */
import { describe, it, expect, beforeEach, afterEach } from "vite-plus/test";
import { QueryClient } from "@tanstack/react-query";
import type { ApiAdapter, User, Workspace } from "@neokapi/ui";
import { createBowrainRouter } from "./index";
import { AnalyticsEvents } from "../analytics-events";
import type { PlatformAdapter } from "../platform";
import { readIntendedPlan, stashIntendedPlan, type IntendedPlan } from "./intended-plan";

type AnyThrown = { options: { to: string; params?: unknown; search?: unknown; replace?: boolean } };

const WORKSPACE: Workspace = {
  id: "ws-1",
  name: "Acme",
  slug: "acme",
  description: "",
  logo_url: "",
  type: "personal",
};

const ONBOARDED: User = {
  id: "u-1",
  email: "buyer@example.com",
  name: "Buyer",
  avatar_url: "",
  onboarded_at: "2026-08-01T00:00:00Z",
};

interface CapturedEvent {
  event: string;
  props?: Record<string, unknown>;
  options?: { transport?: "beacon" };
}

interface Bootstrap {
  mode?: "server" | "standalone";
  user: User | null;
  captured?: CapturedEvent[];
  /**
   * "never" reproduces `fetchJSON`'s unrecoverable-401 branch, which calls
   * `onSessionExpired` and then returns a promise that blocks forever while the
   * page leaves for the identity provider. That is the real behaviour on the
   * anonymous path, and the reason nothing may await it before stashing.
   */
  workspaces?: Workspace[] | "never";
}

function fakeApi(b: Bootstrap): ApiAdapter {
  return {
    getConfig: async () => ({ mode: b.mode ?? "server" }),
    getCurrentUser: async () => b.user,
    listWorkspaces: () =>
      b.workspaces === "never"
        ? new Promise<Workspace[]>(() => {})
        : Promise.resolve(b.workspaces ?? [WORKSPACE]),
  } as unknown as ApiAdapter;
}

function indexBeforeLoad(b: Bootstrap) {
  const api = fakeApi(b);
  const analytics = b.captured
    ? ({
        capture: (
          event: string,
          props?: Record<string, unknown>,
          options?: { transport?: "beacon" },
        ) => b.captured!.push({ event, props, options }),
      } as unknown as PlatformAdapter["analytics"])
    : undefined;
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const route = createBowrainRouter({ queryClient, api, analytics }).routesById["/"] as unknown as {
    options: { beforeLoad: (opts: unknown) => Promise<unknown> };
  };
  return (search: { plan?: IntendedPlan; seats?: number }) =>
    route.options.beforeLoad({ context: { queryClient, api, analytics }, search });
}

/** Run `beforeLoad` and return whatever it throws, or undefined if it resolves. */
async function thrownBy(run: Promise<unknown>): Promise<AnyThrown | undefined> {
  try {
    await run;
  } catch (err) {
    return err as AnyThrown;
  }
  return undefined;
}

/** Let the bootstrap microtasks drain without waiting on a promise that hangs. */
const drain = () => new Promise((resolve) => setTimeout(resolve, 0));

function wipeCookies() {
  for (const c of document.cookie.split("; ")) {
    const name = c.split("=")[0];
    if (name) document.cookie = `${name}=; path=/; max-age=0`;
  }
}

describe("index route: the intended plan survives the identity-provider hop", () => {
  const realLocation = window.location;

  beforeEach(() => {
    wipeCookies();
    Object.defineProperty(window, "location", {
      configurable: true,
      writable: true,
      value: Object.assign({}, realLocation, { href: "https://app.example/" }),
    });
  });

  afterEach(() => {
    Object.defineProperty(window, "location", { configurable: true, value: realLocation });
    wipeCookies();
  });

  it("stashes before awaiting anything, so a hung bootstrap cannot swallow it", () => {
    const run = indexBeforeLoad({ user: null, workspaces: "never" });
    const pending = run({ plan: "pro", seats: 3 });
    void pending.catch(() => {});

    // Asserted synchronously: not one microtask has run, so this passes only
    // while the stash sits ahead of every await in `beforeLoad`.
    expect(readIntendedPlan()).toEqual({ plan: "pro", seats: 3 });
  });

  it("reaches its own login redirect for an anonymous visitor", async () => {
    const run = indexBeforeLoad({ user: null, workspaces: "never" });
    const pending = run({ plan: "pro" });
    void pending.catch(() => {});

    const outcome = await Promise.race([
      pending.then(() => "settled").catch(() => "rejected"),
      drain().then(() => "pending"),
    ]);

    // The route holds the render open while the browser navigates away, and it
    // gets there without the workspaces fetch ever settling.
    expect(outcome).toBe("pending");
    expect(window.location.href).toBe("/api/v1/auth/login");
    expect(readIntendedPlan()).toEqual({ plan: "pro", seats: undefined });
  });

  it("honours a stash carried back from the identity provider", async () => {
    stashIntendedPlan({ plan: "team", seats: 5 });

    const run = indexBeforeLoad({ user: ONBOARDED });
    const thrown = await thrownBy(run({}));

    expect(thrown, "a returning buyer must land on billing, not the dashboard").toBeDefined();
    expect(thrown!.options.to).toBe("/$workspace/settings/billing");
    expect(thrown!.options.params).toEqual({ workspace: "acme" });
    expect(thrown!.options.search).toEqual({ plan: "team", seats: 5 });
    expect(readIntendedPlan(), "consumed, so a later plain visit is not hijacked").toBeNull();
  });

  it("routes a signed-in visitor's fresh ?plan straight to billing", async () => {
    const run = indexBeforeLoad({ user: ONBOARDED });
    const thrown = await thrownBy(run({ plan: "pro" }));

    expect(thrown!.options.to).toBe("/$workspace/settings/billing");
    expect(thrown!.options.search).toEqual({ plan: "pro", seats: undefined });
    expect(readIntendedPlan()).toBeNull();
  });

  it("leaves no stash behind in standalone, which has no billing surface", async () => {
    const run = indexBeforeLoad({ mode: "standalone", user: null, workspaces: "never" });
    const thrown = await thrownBy(run({ plan: "pro" }));

    expect(thrown!.options.to).toBe("/$workspace");
    expect(thrown!.options.params).toEqual({ workspace: "local" });
    expect(readIntendedPlan()).toBeNull();
  });

  it("takes a first-run buyer through onboarding with the plan still stashed", async () => {
    const run = indexBeforeLoad({
      user: { ...ONBOARDED, onboarded_at: null },
      workspaces: [],
    });
    const thrown = await thrownBy(run({ plan: "team", seats: 2 }));

    // /welcome consumes the stash once the workspace exists (WelcomeRoute).
    expect(thrown!.options.to).toBe("/welcome");
    expect(readIntendedPlan()).toEqual({ plan: "team", seats: 2 });
  });

  // No route renders for a visitor with no session, so the app's own $pageview
  // never fires and the hop to the identity provider — the most expensive step
  // in the funnel — is the one step with no data.
  it("marks the conversion step before leaving for the identity provider", async () => {
    const captured: CapturedEvent[] = [];
    const run = indexBeforeLoad({ user: null, workspaces: "never", captured });
    void run({ plan: "pro" }).catch(() => {});

    await drain();

    expect(captured).toHaveLength(1);
    expect(captured[0].event).toBe(AnalyticsEvents.signupRedirectStarted);
    expect(captured[0].props).toEqual({ has_plan: true, plan: "pro" });
    // The navigation below discards a batched payload, so this one goes out
    // through the transport that survives the unload.
    expect(captured[0].options).toEqual({ transport: "beacon" });
    expect(window.location.href).toBe("/api/v1/auth/login");
  });

  it("marks the conversion step for a visitor who chose no plan", async () => {
    const captured: CapturedEvent[] = [];
    const run = indexBeforeLoad({ user: null, workspaces: "never", captured });
    void run({}).catch(() => {});

    await drain();

    expect(captured).toHaveLength(1);
    expect(captured[0].props).toEqual({ has_plan: false, plan: undefined });
  });

  it("emits nothing for a visitor who already has a session", async () => {
    const captured: CapturedEvent[] = [];
    const run = indexBeforeLoad({ user: ONBOARDED, captured });
    await thrownBy(run({}));

    expect(captured).toEqual([]);
  });

  it("writes nothing when no plan was asked for", async () => {
    const run = indexBeforeLoad({ user: ONBOARDED });
    const thrown = await thrownBy(run({}));

    expect(thrown!.options.to).toBe("/$workspace");
    expect(readIntendedPlan()).toBeNull();
  });
});
