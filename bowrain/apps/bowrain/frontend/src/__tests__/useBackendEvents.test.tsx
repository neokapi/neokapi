import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, act } from "@testing-library/react";
import { BackendEventsProvider, useBackendEvents } from "../hooks/useBackendEvents";

// Controllable Wails Events bus: capture handlers registered via Events.On so
// the test can fire backend events synchronously.
const handlers = new Map<string, Set<(e: { data: unknown }) => void>>();

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, handler: (e: { data: unknown }) => void) => {
      let set = handlers.get(name);
      if (!set) {
        set = new Set();
        handlers.set(name, set);
      }
      set.add(handler);
      return () => set!.delete(handler);
    },
    Emit: vi.fn(),
  },
}));

function fireBackendEvent(name: string, data: unknown) {
  act(() => {
    for (const h of handlers.get(name) ?? []) h({ data });
  });
}

describe("useBackendEvents", () => {
  beforeEach(() => handlers.clear());

  it("invokes the listener when its subscribed event fires", () => {
    const onBlocks = vi.fn();
    function View() {
      useBackendEvents("blocks-changed", onBlocks);
      return null;
    }
    render(
      <BackendEventsProvider>
        <View />
      </BackendEventsProvider>,
    );

    fireBackendEvent("blocks-changed", { event_type: "block.updated" });
    expect(onBlocks).toHaveBeenCalledTimes(1);
    expect(onBlocks).toHaveBeenCalledWith({ event_type: "block.updated" });
  });

  it("does not invoke a listener for an unrelated event", () => {
    const onBlocks = vi.fn();
    function View() {
      useBackendEvents("blocks-changed", onBlocks);
      return null;
    }
    render(
      <BackendEventsProvider>
        <View />
      </BackendEventsProvider>,
    );

    fireBackendEvent("connector-sync", { event_type: "connector.sync.completed" });
    expect(onBlocks).not.toHaveBeenCalled();
  });

  it("supports an array of events", () => {
    const handler = vi.fn();
    function View() {
      useBackendEvents(["project-changed", "connector-sync"], handler);
      return null;
    }
    render(
      <BackendEventsProvider>
        <View />
      </BackendEventsProvider>,
    );

    fireBackendEvent("project-changed", { event_type: "project.updated" });
    fireBackendEvent("connector-sync", { event_type: "connector.sync.completed" });
    expect(handler).toHaveBeenCalledTimes(2);
  });

  it("re-runs every refreshable listener on reconnect", () => {
    const onBlocks = vi.fn();
    const onProject = vi.fn();
    const onConnector = vi.fn();
    function View() {
      useBackendEvents("blocks-changed", onBlocks);
      useBackendEvents("project-changed", onProject);
      useBackendEvents("connector-sync", onConnector);
      return null;
    }
    render(
      <BackendEventsProvider>
        <View />
      </BackendEventsProvider>,
    );

    fireBackendEvent("reconnected", { state: "connected" });

    // A reconnect forces a full refresh of all open views.
    expect(onBlocks).toHaveBeenCalledTimes(1);
    expect(onProject).toHaveBeenCalledTimes(1);
    expect(onConnector).toHaveBeenCalledTimes(1);
  });

  it("unsubscribes on unmount", () => {
    const onBlocks = vi.fn();
    function View() {
      useBackendEvents("blocks-changed", onBlocks);
      return null;
    }
    const { unmount } = render(
      <BackendEventsProvider>
        <View />
      </BackendEventsProvider>,
    );

    unmount();
    fireBackendEvent("blocks-changed", { event_type: "block.updated" });
    expect(onBlocks).not.toHaveBeenCalled();
  });

  it("is a no-op without a provider (storybook/isolation safety)", () => {
    const onBlocks = vi.fn();
    function View() {
      useBackendEvents("blocks-changed", onBlocks);
      return null;
    }
    // No provider — must not throw.
    expect(() => render(<View />)).not.toThrow();
  });
});
