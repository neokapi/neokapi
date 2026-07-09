import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ApiAdapter, Membership } from "@neokapi/ui";
import { MembersPage } from "./MembersPage";
import { withWorkspaceApi, mockApiAdapter, mockPersonalWorkspace } from "../storybook/backend";

/**
 * MembersPage is a `useApi()`/`useWorkspace()` container — the desktop's
 * workspace-membership governance surface. Its react-query `queryFn`s and the
 * embedded InviteManager both read through the `ApiAdapter`, so the stories back
 * it with a mock adapter through `withWorkspaceApi`; no Wails runtime involved.
 */

const members: Membership[] = [
  {
    user_id: "u-owner",
    workspace_id: "ws-acme",
    role: "owner",
    user: { id: "u-owner", email: "olive@acme.test", name: "Olive Owner", avatar_url: "" },
  },
  {
    user_id: "u-admin",
    workspace_id: "ws-acme",
    role: "admin",
    user: { id: "u-admin", email: "ada@acme.test", name: "Ada Admin", avatar_url: "" },
  },
  {
    user_id: "u-member",
    workspace_id: "ws-acme",
    role: "member",
    user: { id: "u-member", email: "mel@acme.test", name: "Mel Member", avatar_url: "" },
  },
];

function membersAdapter(): ApiAdapter {
  return mockApiAdapter({
    listMembers: () => Promise.resolve(members),
    updateMemberRole: () => Promise.resolve(undefined),
    removeMember: () => Promise.resolve(undefined),
    listInvites: () => Promise.resolve([]),
    createInvite: () => Promise.resolve(undefined),
    deleteInvite: () => Promise.resolve(undefined),
  } as unknown as Partial<ApiAdapter>);
}

const meta: Meta<typeof MembersPage> = {
  title: "Bowrain/MembersPage",
  component: MembersPage,
  tags: ["autodocs"],
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof MembersPage>;

/** A populated roster in a connected team workspace (owner can manage roles). */
export const Roster: Story = {
  decorators: [withWorkspaceApi({ adapter: membersAdapter() })],
};

/** The disconnected personal workspace shows the connect prompt. */
export const PersonalWorkspace: Story = {
  decorators: [withWorkspaceApi({ adapter: membersAdapter(), workspace: mockPersonalWorkspace })],
};
