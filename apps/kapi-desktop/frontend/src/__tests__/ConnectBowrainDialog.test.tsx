import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { ConnectBowrainDialog, type ConnectApi } from "../components/ConnectBowrainDialog";
import type { BowrainConnection } from "../types/api";

const DISCONNECTED: BowrainConnection = {
  connected: false,
  server_url: "",
  project_url: "",
  project_id: "",
  authenticated: false,
  user_email: "",
};

function makeApi(overrides: Partial<ConnectApi> = {}): ConnectApi {
  return {
    getBowrainConnection: vi.fn().mockResolvedValue(DISCONNECTED),
    connectBowrain: vi.fn().mockResolvedValue({ server_url: "https://b.example", user_email: "" }),
    publishBowrain: vi
      .fn()
      .mockResolvedValue({ project_id: "", project_url: "", delegated: true, sync_hint: "" }),
    disconnectBowrain: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
}

describe("ConnectBowrainDialog", () => {
  it("shows the server input and a connect button when disconnected and saved", async () => {
    const api = makeApi();
    render(<ConnectBowrainDialog tabID="t1" saved onClose={() => {}} api={api} />);

    await waitFor(() => expect(api.getBowrainConnection).toHaveBeenCalledWith("t1"));
    expect(screen.getByLabelText("Bowrain server")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Connect & sign in/i })).toBeEnabled();
  });

  it("disables connect until the project is saved", async () => {
    const api = makeApi();
    render(<ConnectBowrainDialog tabID="t1" saved={false} onClose={() => {}} api={api} />);

    await waitFor(() => expect(api.getBowrainConnection).toHaveBeenCalled());
    expect(screen.getByRole("button", { name: /Connect & sign in/i })).toBeDisabled();
    expect(screen.getByText(/Save the project to disk/i)).toBeInTheDocument();
  });

  it("drives connect → publish and surfaces the delegated sync hint", async () => {
    const user = userEvent.setup();
    const connected: BowrainConnection = {
      ...DISCONNECTED,
      connected: true,
      authenticated: true,
      server_url: "https://b.example",
      user_email: "dev@example.com",
    };
    // First load = disconnected; after connect = connected.
    const getConn = vi.fn().mockResolvedValueOnce(DISCONNECTED).mockResolvedValue(connected);
    const api = makeApi({
      getBowrainConnection: getConn,
      connectBowrain: vi
        .fn()
        .mockResolvedValue({ server_url: "https://b.example", user_email: "dev@example.com" }),
      publishBowrain: vi.fn().mockResolvedValue({
        project_id: "proj1",
        project_url: "https://b.example/acme/proj1",
        delegated: true,
        sync_hint: "Push content with `kapi sync`.",
      }),
    });

    render(<ConnectBowrainDialog tabID="t1" saved onClose={() => {}} api={api} />);

    const input = screen.getByLabelText("Bowrain server");
    await user.clear(input);
    await user.type(input, "https://b.example");
    await user.click(screen.getByRole("button", { name: /Connect & sign in/i }));

    await waitFor(() => expect(api.connectBowrain).toHaveBeenCalledWith("t1", "https://b.example"));

    // Now in the connected state: a Publish button appears.
    const publishBtn = await screen.findByRole("button", { name: /^Publish$/i });
    await user.click(publishBtn);

    await waitFor(() => expect(api.publishBowrain).toHaveBeenCalledWith("t1"));
    expect(await screen.findByText(/Published to Bowrain/i)).toBeInTheDocument();
    expect(screen.getByText(/Push content with `kapi sync`/i)).toBeInTheDocument();
    expect(screen.getByText("https://b.example/acme/proj1")).toBeInTheDocument();
  });

  it("surfaces connect errors and returns to the idle state", async () => {
    const user = userEvent.setup();
    const api = makeApi({
      connectBowrain: vi.fn().mockRejectedValue(new Error("authenticate: login timed out")),
    });
    render(<ConnectBowrainDialog tabID="t1" saved onClose={() => {}} api={api} />);

    await user.click(screen.getByRole("button", { name: /Connect & sign in/i }));
    expect(await screen.findByText(/login timed out/i)).toBeInTheDocument();
    // Back to idle: the server input is still shown.
    expect(screen.getByLabelText("Bowrain server")).toBeInTheDocument();
  });

  it("opens directly into the connected state when already authenticated", async () => {
    const connected: BowrainConnection = {
      ...DISCONNECTED,
      connected: true,
      authenticated: true,
      server_url: "https://b.example",
      user_email: "dev@example.com",
    };
    const api = makeApi({ getBowrainConnection: vi.fn().mockResolvedValue(connected) });
    render(<ConnectBowrainDialog tabID="t1" saved onClose={() => {}} api={api} />);

    expect(await screen.findByRole("button", { name: /^Publish$/i })).toBeInTheDocument();
    expect(screen.getByText("dev@example.com")).toBeInTheDocument();
  });

  it("can disconnect from the connected state", async () => {
    const user = userEvent.setup();
    const connected: BowrainConnection = {
      ...DISCONNECTED,
      connected: true,
      authenticated: true,
      server_url: "https://b.example",
    };
    const disconnect = vi.fn().mockResolvedValue(undefined);
    const api = makeApi({
      getBowrainConnection: vi.fn().mockResolvedValue(connected),
      disconnectBowrain: disconnect,
    });
    render(<ConnectBowrainDialog tabID="t1" saved onClose={() => {}} api={api} />);

    await user.click(await screen.findByRole("button", { name: /Disconnect/i }));
    await waitFor(() => expect(disconnect).toHaveBeenCalledWith("t1"));
  });
});
