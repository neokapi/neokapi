import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConnectionIndicator } from "../components/ConnectionIndicator";

describe("ConnectionIndicator", () => {
  it("says nothing while connected with a clean queue", () => {
    const { container } = render(
      <ConnectionIndicator connectionState="connected" pendingChanges={0} failedChanges={0} />,
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

  it("keeps showing rejected edits after the connection is back", () => {
    render(<ConnectionIndicator connectionState="connected" failedChanges={2} />);
    expect(screen.getByTestId("connection-failed")).toHaveTextContent("2 failed");
  });
});
