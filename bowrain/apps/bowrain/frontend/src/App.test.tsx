import { render, screen, fireEvent, act, cleanup } from "@testing-library/react";
import type { ComponentType } from "react";
import { describe, it, expect, vi, beforeAll, beforeEach, afterEach } from "vitest";

// The gate mounts the real ServerConnect / OfflineLaunch screens and a stubbed
// shared app, over mocked Wails bindings + composite adapter, so we can assert
// the launch state machine: reachable → app, unreachable → offline-launch (with
// auto-recovery), no/rejected session → sign-in.

const getConfig = vi.hoisted(() => vi.fn());
const getCurrentUser = vi.hoisted(() => vi.fn());
const backend = vi.hoisted(() => ({
  GetConnectionState: vi.fn(),
  ConnectToServer: vi.fn().mockResolvedValue(undefined),
  Disconnect: vi.fn().mockResolvedValue(undefined),
  Logout: vi.fn().mockResolvedValue(undefined),
  GetDefaultServerURL: vi.fn().mockResolvedValue("https://app.bowrain.cloud"),
  StartLogin: vi.fn().mockResolvedValue(undefined),
  WaitForLogin: vi.fn().mockResolvedValue(false),
  CancelLogin: vi.fn().mockResolvedValue(undefined),
  SelectWorkspace: vi.fn().mockResolvedValue(undefined),
  GetServerWorkspaces: vi.fn().mockResolvedValue([]),
}));

// A stand-in for the composite adapter's typed connectivity error — App checks
// `instanceof ProxyConnectivityError` against this same (mocked) class.
class FakeProxyConnectivityError extends Error {
  constructor() {
    super("server unreachable");
    this.name = "ProxyConnectivityError";
  }
}

vi.mock("./api/backend", () => ({ Backend: backend }));
vi.mock("./api/desktopAdapter", () => ({
  createDesktopAdapter: () => ({ getConfig, getCurrentUser }),
  ProxyConnectivityError: FakeProxyConnectivityError,
}));
vi.mock("./api/desktopPlatform", () => ({ createDesktopPlatform: () => ({ kind: "desktop" }) }));
vi.mock("@neokapi/bowrain-app", () => ({
  BowrainApp: () => <div data-testid="bowrain-app">app</div>,
}));

// Import the module (and its heavy transitive graph: the shared app, react-query,
// the router) once, up front. The first dynamic import pays a one-time Vite
// transform cost that, under CPU load, would otherwise land inside — and blow —
// the first test's timeout. Warming it here keeps every test body cheap.
let App: ComponentType;
beforeAll(async () => {
  ({ default: App } = await import("./App"));
});

beforeEach(() => {
  vi.clearAllMocks();
  // Fake timers make the launch flow deterministic. The gate chains several async
  // transitions (GetConnectionState → getConfig → getCurrentUser → render) and the
  // OfflineLaunch auto-retry runs on a setTimeout backoff. With real timers the
  // terminal waitFor raced a wall-clock 5s deadline — under CI CPU load its own
  // poll/timeout timers were starved and the chain settled too late, timing out
  // (#1306). Driving a mock clock via flushUntil() advances every pending
  // microtask and timer explicitly, so the observed outcome no longer depends on
  // how fast the machine is.
  vi.useFakeTimers();
  // Default: a valid session. Individual tests override (null = signed out).
  getCurrentUser.mockResolvedValue({ id: "u1", name: "Alex", email: "a@x.io" });
});

afterEach(() => {
  // Unmount before restoring real timers so the OfflineLaunch backoff loop's
  // cleanup runs against the fake clock, then drop any timers it left queued.
  // (Cleanup here rather than via auto-cleanup so it precedes useRealTimers.)
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

async function renderApp() {
  let utils!: ReturnType<typeof render>;
  // Mount inside act() so the launch effect's first synchronous updates are
  // flushed before any assertion (the rest are drained by flushUntil).
  await act(async () => {
    utils = render(<App />);
  });
  return utils;
}

// Deterministic replacement for waitFor under fake timers. A single act() scope
// (no overlapping act() calls) drains the launch chain: each iteration advances
// the mock clock, which flushes the pending microtask chain (the launch awaits)
// and fires any scheduled setTimeout (the OfflineLaunch retry backoff, including
// timers queued mid-drain), then checks the predicate. Bounded by iterations, not
// wall-clock, so machine load can't race it. Returns as soon as `predicate`
// holds; throws if it never does.
async function flushUntil(predicate: () => boolean, maxIterations = 100) {
  let satisfied = false;
  await act(async () => {
    for (let i = 0; i < maxIterations; i++) {
      // Settle the pending microtask chain (the launch awaits) and fire any
      // scheduled setTimeout — the OfflineLaunch retry backoff, including timers
      // queued mid-drain. advanceTimersByTimeAsync flushes microtasks between
      // each fired timer, so both kinds of pending work drain together.
      await vi.advanceTimersByTimeAsync(1000);
      if (predicate()) {
        satisfied = true;
        return;
      }
    }
  });
  if (!satisfied) {
    throw new Error(`flushUntil: predicate not satisfied after ${maxIterations} iterations`);
  }
}

describe("desktop launch gate (#1284)", () => {
  it("enters the app when the server is reachable", async () => {
    backend.GetConnectionState.mockResolvedValue({
      state: "connected",
      server_url: "https://acme.test",
      workspace: "w",
    });
    getConfig.mockResolvedValue({ mode: "server" });

    await renderApp();
    await flushUntil(() => screen.queryByTestId("bowrain-app") !== null);

    expect(screen.getByTestId("bowrain-app")).toBeInTheDocument();
  });

  it("shows the offline-launch state (not sign-in, not the app) when the server is unreachable", async () => {
    backend.GetConnectionState.mockResolvedValue({
      state: "connected",
      server_url: "https://acme.test",
    });
    getConfig.mockRejectedValue(new FakeProxyConnectivityError());

    await renderApp();
    await flushUntil(() => screen.queryByText(/can't reach your server/i) !== null);

    expect(screen.getByText(/can't reach your server/i)).toBeInTheDocument();
    expect(screen.getByText("https://acme.test")).toBeInTheDocument();
    expect(screen.queryByTestId("bowrain-app")).toBeNull();
    expect(screen.queryByText(/welcome to bowrain/i)).toBeNull();
  });

  it("auto-recovers into the app when connectivity returns — no user action", async () => {
    backend.GetConnectionState.mockResolvedValue({
      state: "connected",
      server_url: "https://acme.test",
    });
    // Unreachable on the launch probe, reachable on the automatic retry.
    getConfig
      .mockRejectedValueOnce(new FakeProxyConnectivityError())
      .mockResolvedValue({ mode: "server" });

    await renderApp();

    // First probe lands in the offline-launch state, then the backoff retry
    // (fired by the advancing mock clock) reaches the server and enters the app.
    await flushUntil(() => screen.queryByText(/can't reach your server/i) !== null);
    await flushUntil(() => screen.queryByTestId("bowrain-app") !== null);

    expect(screen.getByTestId("bowrain-app")).toBeInTheDocument();
  });

  it("shows sign-in when there is no stored session", async () => {
    backend.GetConnectionState.mockResolvedValue({ state: "disconnected" });

    await renderApp();
    await flushUntil(() => screen.queryByText(/welcome to bowrain/i) !== null);

    expect(screen.getByText(/welcome to bowrain/i)).toBeInTheDocument();
    expect(screen.queryByText(/can't reach your server/i)).toBeNull();
  });

  it("routes a rejected/expired session to sign-in, not the offline state", async () => {
    // Optimistically connected and the server is reachable (getConfig is public),
    // but the stored token is rejected — getCurrentUser returns null. That's the
    // sign-in signal; the gate must clear the token and show ServerConnect.
    backend.GetConnectionState.mockResolvedValue({
      state: "connected",
      server_url: "https://acme.test",
    });
    getConfig.mockResolvedValue({ mode: "server" });
    getCurrentUser.mockResolvedValue(null);

    await renderApp();
    await flushUntil(() => screen.queryByText(/welcome to bowrain/i) !== null);

    expect(screen.getByText(/welcome to bowrain/i)).toBeInTheDocument();
    expect(backend.Logout).toHaveBeenCalled();
    expect(screen.queryByText(/can't reach your server/i)).toBeNull();
    expect(screen.queryByTestId("bowrain-app")).toBeNull();
  });

  it("'Use a different server' drops the connection and shows sign-in", async () => {
    backend.GetConnectionState.mockResolvedValue({
      state: "connected",
      server_url: "https://acme.test",
    });
    getConfig.mockRejectedValue(new FakeProxyConnectivityError());

    await renderApp();
    await flushUntil(() => screen.queryByText(/can't reach your server/i) !== null);

    // Disconnecting drops the optimistic connection (backend now disconnected).
    backend.Disconnect.mockImplementation(async () => {
      backend.GetConnectionState.mockResolvedValue({ state: "disconnected" });
    });
    await act(async () => {
      fireEvent.click(screen.getByTestId("offline-use-different-server"));
    });
    await flushUntil(() => screen.queryByText(/welcome to bowrain/i) !== null);

    expect(backend.Disconnect).toHaveBeenCalled();
    expect(screen.getByText(/welcome to bowrain/i)).toBeInTheDocument();
  });
});
