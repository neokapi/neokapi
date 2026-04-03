import type { Meta, StoryObj } from "@storybook/react-vite";
import { ApiTokenManager } from "../../components/ApiTokenManager";
import { withProviders, createProvidersDecorator } from "../decorators";
import { sampleWorkspace, viewerWorkspace } from "./fixtures";
import type { ApiToken } from "../../types/api";

const sampleTokens: ApiToken[] = [
  {
    id: "tok-1",
    user_id: "u-1",
    workspace_id: "ws-1",
    name: "CI/CD Pipeline",
    token_prefix: "bwt_ab12",
    scopes: '["*"]',
    last_used_at: new Date(Date.now() - 86400000).toISOString(),
    expires_at: new Date(Date.now() + 30 * 86400000).toISOString(),
    created_at: new Date(Date.now() - 7 * 86400000).toISOString(),
  },
  {
    id: "tok-2",
    user_id: "u-1",
    workspace_id: "ws-1",
    name: "Translation Bot",
    token_prefix: "bwt_cd34",
    scopes: '["translate:fr,de"]',
    last_used_at: null,
    expires_at: new Date(Date.now() + 90 * 86400000).toISOString(),
    created_at: new Date(Date.now() - 2 * 86400000).toISOString(),
  },
  {
    id: "tok-3",
    user_id: "u-1",
    workspace_id: "ws-1",
    name: "Review Service",
    token_prefix: "bwt_ef56",
    scopes: '["review"]',
    last_used_at: new Date(Date.now() - 3600000).toISOString(),
    expires_at: null,
    created_at: new Date(Date.now() - 14 * 86400000).toISOString(),
  },
  {
    id: "tok-4",
    user_id: "u-1",
    workspace_id: "ws-1",
    name: "Read-Only Monitor",
    token_prefix: "bwt_gh78",
    scopes: '["read"]',
    last_used_at: null,
    expires_at: new Date(Date.now() - 86400000).toISOString(), // expired
    created_at: new Date(Date.now() - 60 * 86400000).toISOString(),
  },
];

const withTokens = createProvidersDecorator(undefined, {
  listApiTokens: async () => sampleTokens,
});

const meta: Meta<typeof ApiTokenManager> = {
  title: "Workspace/Access/ApiTokenManager",
  component: ApiTokenManager,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 800, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ApiTokenManager>;

/** Owner view with empty token list. */
export const OwnerView: Story = {
  decorators: [withProviders],
  args: { workspace: sampleWorkspace },
};

/** Owner view with tokens showing various scopes. */
export const WithTokens: Story = {
  decorators: [withTokens],
  args: { workspace: sampleWorkspace },
};

/** Viewer — component returns null since role is not owner/admin. */
export const ViewerHidden: Story = {
  decorators: [withProviders],
  args: { workspace: viewerWorkspace },
};
