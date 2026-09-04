import type { Meta, StoryObj } from "@storybook/react-vite";
import { ErrorNotice } from "./ErrorNotice";
import { apiErrorFromResponse } from "./ApiError";

const meta: Meta<typeof ErrorNotice> = {
  title: "Foundations/ErrorNotice",
  component: ErrorNotice,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 560, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ErrorNotice>;

/** Panel variant with a REST `{error, details}` envelope and Details disclosure. */
export const Panel: Story = {
  args: {
    error: {
      error: "invalid request body",
      details: "field default_source_language is required",
    },
    title: "Couldn't create the project",
  },
};

/** Panel with a known code mapped to friendly phrasing + recovery hint. */
export const PanelWithCode: Story = {
  args: {
    error: { error: "cannot grant permissions beyond your own", code: "forbidden" },
    title: "Couldn't save the member role",
  },
};

/**
 * A 403 that states its reason: the review surface's banner when the server
 * refuses the reviewer's own work. The sentence stands alone; no grant fixes
 * it, so no remedy is appended.
 */
export const RefusedReviewAction: Story = {
  args: {
    variant: "inline",
    error: apiErrorFromResponse(
      403,
      JSON.stringify({
        error: "separation of duties: you cannot review or approve your own work",
        reference: "req_3k9d2p",
      }),
    ),
    title: "The review action didn't go through",
  },
};

/** A 403 written for a missing grant keeps the remedy a workspace admin can act on. */
export const PermissionDenied: Story = {
  args: {
    error: apiErrorFromResponse(
      403,
      JSON.stringify({ error: "insufficient project permissions", reference: "req_7q1mz0" }),
    ),
    title: "Couldn't approve the translation",
  },
};

/** The RestApiAdapter pattern: `Error("<status>: <json body>")`. */
export const AdapterError: Story = {
  args: {
    error: new Error(
      '409: {"error":"slug is already in use","details":"workspace slug \\"acme\\" was taken 3 days ago"}',
    ),
    title: "Couldn't rename the workspace",
  },
};

/** Expanded highlighted-JSON details (open the disclosure to compare themes). */
export const PanelDetailsOpen: Story = {
  render: () => (
    <ErrorNoticeOpen
      error={{
        error: "rate limit exceeded: max 10 pushes per minute per project",
        code: "rate_limited",
        details: "retry after 42s",
        request_id: "req_8fk2m1",
        limits: { per_minute: 10, used: 10 },
      }}
      title="Push rejected"
    />
  ),
};

/** Compact inline variant for dialogs and forms. */
export const Inline: Story = {
  args: {
    variant: "inline",
    error: { error: "stream is required" },
  },
};

/** Inline variant with an expanded JSON detail view. */
export const InlineDetailsOpen: Story = {
  render: () => (
    <ErrorNoticeOpen
      variant="inline"
      error={new Error('404: {"error":"block not found","block_id":"b_1289"}')}
    />
  ),
};

/** Plain-string errors get no disclosure — just the friendly line. */
export const PlainString: Story = {
  args: {
    error: "Failed to reach the translation service",
    hint: "Check your connection and try again.",
  },
};

/** With a retry action. */
export const WithRetry: Story = {
  args: {
    error: new Error("503: "),
    title: "Couldn't load activities",
    onRetry: () => {},
  },
};

/** Helper that renders ErrorNotice with the Details disclosure pre-opened. */
function ErrorNoticeOpen(props: React.ComponentProps<typeof ErrorNotice>) {
  return (
    <div
      ref={(node) => {
        // Open the disclosure once mounted so stories/screenshots show the JSON.
        const btn = node?.querySelector<HTMLButtonElement>(
          '[data-testid="error-notice-details-toggle"][aria-expanded="false"]',
        );
        btn?.click();
      }}
    >
      <ErrorNotice {...props} />
    </div>
  );
}
