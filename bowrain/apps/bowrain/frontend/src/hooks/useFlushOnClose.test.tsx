import { act, fireEvent, render, renderHook, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vite-plus/test";
import { Events } from "@wailsio/runtime";

const backend = vi.hoisted(() => ({
  SetPendingSaves: vi.fn().mockResolvedValue(undefined),
  AcknowledgeClose: vi.fn().mockResolvedValue(undefined),
  ConfirmClose: vi.fn().mockResolvedValue(undefined),
  CancelClose: vi.fn().mockResolvedValue(undefined),
}));
const saves = vi.hoisted(() => ({
  pending: false,
  listeners: new Set<() => void>(),
  flush: vi.fn(),
}));

vi.mock("../api/backend", () => ({ Backend: backend }));
vi.mock("@neokapi/bowrain-app", () => ({
  hasPendingSaves: () => saves.pending,
  subscribePendingSaves: (listener: () => void) => {
    saves.listeners.add(listener);
    return () => saves.listeners.delete(listener);
  },
  flushPendingSaves: (timeoutMs: number) => saves.flush(timeoutMs),
}));

import { CLOSE_REQUESTED_EVENT, FLUSH_TIMEOUT_MS, useFlushOnClose } from "./useFlushOnClose";
import { SaveFailedOnClose } from "../components/SaveFailedOnClose";

/** Delivers a backend event to the handlers the hook registered. */
function emit(name: string) {
  for (const [event, handler] of vi.mocked(Events.On).mock.calls) {
    if (event === name) (handler as (e: { data: unknown }) => void)({ data: {} });
  }
}

function setPending(pending: boolean) {
  saves.pending = pending;
  for (const listener of saves.listeners) listener();
}

function Shell() {
  const { failure, closeAnyway, stay } = useFlushOnClose();
  return failure ? (
    <SaveFailedOnClose
      failure={failure}
      onCloseAnyway={() => void closeAnyway()}
      onStay={() => void stay()}
    />
  ) : null;
}

describe("useFlushOnClose", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    saves.pending = false;
    saves.listeners.clear();
  });

  it("tells the backend whether a save is waiting, and stops holding on unmount", () => {
    const { unmount } = renderHook(() => useFlushOnClose());
    expect(backend.SetPendingSaves).toHaveBeenLastCalledWith(false);
    act(() => setPending(true));
    expect(backend.SetPendingSaves).toHaveBeenLastCalledWith(true);
    unmount();
    expect(backend.SetPendingSaves).toHaveBeenLastCalledWith(false);
  });

  it("stops holding the window when the page unloads", () => {
    renderHook(() => useFlushOnClose());
    act(() => setPending(true));
    window.dispatchEvent(new Event("pagehide"));
    expect(backend.SetPendingSaves).toHaveBeenLastCalledWith(false);
  });

  it("on a held close, flushes the waiting saves within the bound, then closes", async () => {
    saves.flush.mockResolvedValue({ ok: true });
    render(<Shell />);
    act(() => setPending(true));
    act(() => emit(CLOSE_REQUESTED_EVENT));
    expect(backend.AcknowledgeClose).toHaveBeenCalled();
    await waitFor(() => expect(backend.ConfirmClose).toHaveBeenCalledTimes(1));
    expect(saves.flush).toHaveBeenCalledWith(FLUSH_TIMEOUT_MS);
    expect(screen.queryByTestId("save-failed-on-close")).toBeNull();
  });

  it("a failed save keeps the window open and shows why", async () => {
    saves.flush.mockResolvedValue({
      ok: false,
      timedOut: false,
      errors: [new Error("409 conflict")],
    });
    render(<Shell />);
    act(() => emit(CLOSE_REQUESTED_EVENT));
    const dialog = await screen.findByTestId("save-failed-on-close");
    expect(dialog).toHaveTextContent("Changes not saved");
    expect(dialog).toHaveTextContent("409 conflict");
    expect(backend.ConfirmClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Stay" }));
    await waitFor(() => expect(backend.CancelClose).toHaveBeenCalledTimes(1));
    expect(backend.ConfirmClose).not.toHaveBeenCalled();
  });

  it("a wait that runs out says so, and Close anyway closes", async () => {
    saves.flush.mockResolvedValue({ ok: false, timedOut: true, errors: [] });
    render(<Shell />);
    act(() => emit(CLOSE_REQUESTED_EVENT));
    const dialog = await screen.findByTestId("save-failed-on-close");
    expect(dialog).toHaveTextContent("The server did not answer in time.");
    fireEvent.click(screen.getByRole("button", { name: "Close anyway" }));
    await waitFor(() => expect(backend.ConfirmClose).toHaveBeenCalledTimes(1));
  });
});
