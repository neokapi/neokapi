import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConnectionIndicator } from "../../components/ConnectionIndicator";

const meta: Meta<typeof ConnectionIndicator> = {
  title: "Components/ConnectionIndicator",
  component: ConnectionIndicator,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ display: "flex", alignItems: "center", gap: 12, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ConnectionIndicator>;

/** The ordinary state: a connected app says nothing about its connection. */
export const Connected: Story = {
  args: { connectionState: "connected", pendingChanges: 0, failedChanges: 0 },
};

/** Offline with nothing queued behind it yet. */
export const Offline: Story = {
  args: { connectionState: "offline", pendingChanges: 0, onRetryConnection: fn() },
};

/** Offline with work waiting. The count is what the recorder waits to see go. */
export const OfflineWithPendingChanges: Story = {
  args: { connectionState: "offline", pendingChanges: 3, onRetryConnection: fn() },
};

/** An attempt is in flight, either on the backoff or because Retry was pressed. */
export const Reconnecting: Story = {
  args: { connectionState: "connecting" },
};

/** Rejected edits outlive the outage, so this shows while connected too. */
export const RejectedChanges: Story = {
  args: { connectionState: "connected", failedChanges: 2 },
};

/** Everything at once: offline, work queued, and earlier edits refused. */
export const OfflineWithRejectedChanges: Story = {
  args: {
    connectionState: "offline",
    pendingChanges: 5,
    failedChanges: 1,
    onRetryConnection: fn(),
  },
};

/** On the web there is no working copy, so the seam reports nothing. */
export const NoConnectivitySeam: Story = {
  args: {},
};
