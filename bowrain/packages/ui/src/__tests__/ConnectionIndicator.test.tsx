import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConnectionIndicator } from "../components/ConnectionIndicator";
import type { FailedChange } from "../types/api";

/** A translation the server refused on replay because someone changed it meanwhile. */
const refused: FailedChange = {
  id: 7,
  status: "failed",
  operation: "change_set",
  edits: [
    {
      op: "set_content",
      item: "hello.txt",
      block: "b1",
      locale: "fr",
      text: "Salut tout le monde",
    },
  ],
  reason: "HTTP 409: the translation moved since you read it",
  queued_at: "2026-10-03T12:00:00Z",
};

/** A save an earlier version queued in a form this one no longer sends. */
const dropped: FailedChange = {
  id: 9,
  status: "dropped",
  operation: "update_block_target",
  edits: [{ op: "set_content", block: "b2", locale: "fr", text: "Bonne nuit" }],
  reason: "queued by an earlier version of Bowrain",
  queued_at: "2026-10-01T09:00:00Z",
};

describe("ConnectionIndicator", () => {
  it("says nothing while connected with a clean queue", () => {
    const { container } = render(
      <ConnectionIndicator connectionState="connected" pendingChanges={0} failedChanges={[]} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("says nothing on the web, where the seam reports no state", () => {
    const { container } = render(<ConnectionIndicator />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the offline state with no queued work", () => {
    render(<ConnectionIndicator connectionState="offline" pendingChanges={0} />);
    expect(screen.getByTestId("connection-offline")).toBeInTheDocument();
    expect(screen.getByText("Offline")).toBeInTheDocument();
    expect(screen.queryByTestId("offline-pending")).not.toBeInTheDocument();
  });

  it("counts the queued edits while offline", () => {
    render(<ConnectionIndicator connectionState="offline" pendingChanges={3} />);
    expect(screen.getByTestId("offline-pending")).toHaveTextContent("3 pending");
  });

  it("drops the offline indicator once the connection is back", () => {
    const { rerender } = render(
      <ConnectionIndicator connectionState="offline" pendingChanges={3} />,
    );
    expect(screen.getByTestId("offline-pending")).toBeInTheDocument();

    rerender(<ConnectionIndicator connectionState="connected" pendingChanges={0} />);
    expect(screen.queryByTestId("offline-pending")).not.toBeInTheDocument();
    expect(screen.queryByTestId("connection-offline")).not.toBeInTheDocument();
  });

  it("reports an attempt in flight", () => {
    render(<ConnectionIndicator connectionState="connecting" />);
    expect(screen.getByTestId("connection-reconnecting")).toHaveTextContent("Reconnecting");
    expect(screen.queryByTestId("connection-offline")).not.toBeInTheDocument();
  });

  it("shows the disconnected state as neither offline nor reconnecting", () => {
    const { container } = render(<ConnectionIndicator connectionState="disconnected" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("offers Retry only when the host can attempt one", async () => {
    const user = userEvent.setup();
    const onRetryConnection = vi.fn();
    const { rerender } = render(
      <ConnectionIndicator connectionState="offline" pendingChanges={1} />,
    );
    expect(screen.queryByTestId("connection-retry")).not.toBeInTheDocument();

    rerender(
      <ConnectionIndicator
        connectionState="offline"
        pendingChanges={1}
        onRetryConnection={onRetryConnection}
      />,
    );
    await user.click(screen.getByTestId("connection-retry"));
    expect(onRetryConnection).toHaveBeenCalledTimes(1);
  });

  it("keeps showing the changes that were not sent after the connection is back", () => {
    render(<ConnectionIndicator connectionState="connected" failedChanges={[refused, dropped]} />);
    expect(screen.getByTestId("connection-failed")).toHaveTextContent("2 not sent");
  });

  it("lists each change that was not sent, with its wording and why", async () => {
    const user = userEvent.setup();
    const onDismiss = vi.fn();
    const onDismissAll = vi.fn();
    render(
      <ConnectionIndicator
        connectionState="connected"
        failedChanges={[refused, dropped]}
        onDismissFailedChange={onDismiss}
        onDismissFailedChanges={onDismissAll}
      />,
    );

    await user.click(screen.getByTestId("connection-failed"));

    const first = screen.getByTestId("failed-change-7");
    expect(first).toHaveTextContent("Translation");
    expect(first).toHaveTextContent("hello.txt");
    expect(first).toHaveTextContent("Salut tout le monde");
    expect(first).toHaveTextContent("HTTP 409: the translation moved since you read it");

    // A dropped entry is a notice, not a refusal: it says what it was and
    // asks for the change to be made again.
    const second = screen.getByTestId("failed-change-9");
    expect(second).toHaveAttribute("data-status", "dropped");
    expect(second).toHaveTextContent("Bonne nuit");
    expect(second).toHaveTextContent("earlier version of Bowrain");
    expect(second).toHaveTextContent("Make the change again.");

    await user.click(screen.getByTestId("failed-change-dismiss-9"));
    expect(onDismiss).toHaveBeenCalledWith(9);
    await user.click(screen.getByTestId("failed-changes-dismiss-all"));
    expect(onDismissAll).toHaveBeenCalledTimes(1);
  });
});
