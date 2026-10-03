import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ConnectionIndicator } from "../../components/ConnectionIndicator";
import type { FailedChange } from "../../types/api";

/** The changes an outage left behind, as the desktop's offline queue lists them. */
const failedChanges: FailedChange[] = [
  {
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
  },
  {
    id: 8,
    status: "failed",
    operation: "change_set",
    edits: [{ op: "decide", outcome: "establish", item: "hello.txt", block: "b3", locale: "de" }],
    reason: "HTTP 403: approving takes the review permission for de",
    queued_at: "2026-10-03T12:01:00Z",
  },
  {
    id: 9,
    status: "dropped",
    operation: "update_block_target",
    edits: [{ op: "set_content", block: "b2", locale: "fr", text: "Bonne nuit" }],
    reason: "queued by an earlier version of Bowrain",
    queued_at: "2026-10-01T09:00:00Z",
  },
];

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
  args: { connectionState: "connected", pendingChanges: 0, failedChanges: [] },
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

/**
 * Changes that were not sent outlive the outage, so this shows while connected
 * too. The count opens the list: what each change was, where, its wording and
 * why it was not sent, including a save an earlier version queued in a form
 * this one no longer sends.
 */
export const RejectedChanges: Story = {
  args: {
    connectionState: "connected",
    failedChanges,
    onDismissFailedChange: fn(),
    onDismissFailedChanges: fn(),
  },
};

/** Everything at once: offline, work queued, and earlier edits refused. */
export const OfflineWithRejectedChanges: Story = {
  args: {
    connectionState: "offline",
    pendingChanges: 5,
    failedChanges: failedChanges.slice(0, 1),
    onRetryConnection: fn(),
  },
};

/** On the web there is no working copy, so the seam reports nothing. */
export const NoConnectivitySeam: Story = {
  args: {},
};
