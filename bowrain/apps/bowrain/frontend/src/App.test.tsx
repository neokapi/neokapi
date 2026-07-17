import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

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

beforeEach(() => {
  vi.clearAllMocks();
  // Default: a valid session. Individual tests override (null = signed out).
  getCurrentUser.mockResolvedValue({ id: "u1", name: "Alex", email: "a@x.io" });
});

async function renderApp() {
  const { default: App } = await import("./App");
  return render(<App />);
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

    await waitFor(() => expect(screen.getByTestId("bowrain-app")).toBeInTheDocument());
  });

  it("shows the offline-launch state (not sign-in, not the app) when the server is unreachable", async () => {
    backend.GetConnectionState.mockResolvedValue({
      state: "connected",
      server_url: "https://acme.test",
    });
    getConfig.mockRejectedValue(new FakeProxyConnectivityError());

    await renderApp();

    await waitFor(() => expect(screen.getByText(/can't reach your server/i)).toBeInTheDocument());
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

    await waitFor(() => expect(screen.getByText(/can't reach your server/i)).toBeInTheDocument());
    await waitFor(() => expect(screen.getByTestId("bowrain-app")).toBeInTheDocument(), {
      timeout: 5000,
    });
  });

  it("shows sign-in when there is no stored session", async () => {
    backend.GetConnectionState.mockResolvedValue({ state: "disconnected" });

    await renderApp();

    await waitFor(() => expect(screen.getByText(/welcome to bowrain/i)).toBeInTheDocument());
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

    await waitFor(() => expect(screen.getByText(/welcome to bowrain/i)).toBeInTheDocument());
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
    await waitFor(() => expect(screen.getByText(/can't reach your server/i)).toBeInTheDocument());

    // Disconnecting drops the optimistic connection (backend now disconnected).
    backend.Disconnect.mockImplementation(async () => {
      backend.GetConnectionState.mockResolvedValue({ state: "disconnected" });
    });
    fireEvent.click(screen.getByTestId("offline-use-different-server"));

    await waitFor(() => expect(backend.Disconnect).toHaveBeenCalled());
    await waitFor(() => expect(screen.getByText(/welcome to bowrain/i)).toBeInTheDocument());
  });
});
