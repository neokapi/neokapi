import type { Meta, StoryObj } from "@storybook/react-vite";
import type { PostHogConnectorConfig, PostHogConnectorConfigRequest } from "@neokapi/ui";
import { PostHogConnectCard } from "./PostHogConnectCard";

// Connect PostHog — the card the Locale-demand page opens when the project
// has no analytics source yet. "Test & save" round-trips the server, which
// live-tests the connection before persisting; the stories fake that call.

const savedConfig: PostHogConnectorConfig = {
  configured: true,
  host: "eu",
  host_label: "eu.posthog.com",
  posthog_project_id: "12345",
  api_key_masked: "••••9fk2",
  path_locale_pattern: "^/([a-z]{2}(-[A-Z]{2})?)/",
  updated_at: "2026-07-01T09:12:00Z",
};

const accept = async (req: PostHogConnectorConfigRequest): Promise<PostHogConnectorConfig> => ({
  ...savedConfig,
  host: req.host,
  posthog_project_id: req.posthog_project_id,
});

const rejectWith = (status: number, error: string, code: string) => async () => {
  await new Promise((r) => setTimeout(r, 300));
  throw new Error(`${status}: ${JSON.stringify({ error, code })}`);
};

const meta: Meta<typeof PostHogConnectCard> = {
  title: "Locale Demand/PostHog Connect Card",
  component: PostHogConnectCard,
  parameters: { layout: "centered" },
  args: { onSave: accept },
};

export default meta;
type Story = StoryObj<typeof PostHogConnectCard>;

/** First-time connect: empty form, US Cloud preselected. */
export const Default: Story = {};

/** Re-connect: host/project prefilled, stored key shown as a masked tail. */
export const Reconnect: Story = {
  args: { config: savedConfig },
};

/** Server rejected the personal API key — the error lands on the key field. */
export const BadKey: Story = {
  args: {
    config: { ...savedConfig, api_key_masked: undefined },
    onSave: rejectWith(400, "personal API key was rejected", "bad_key"),
  },
};

/** Self-hosted flow: host select expanded to a URL field. */
export const SelfHosted: Story = {
  args: {
    config: { ...savedConfig, host: "https://posthog.example.com", api_key_masked: "••••x7q1" },
  },
};
