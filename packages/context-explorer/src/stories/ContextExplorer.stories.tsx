import type { Meta, StoryObj } from "@storybook/react-vite";
import { ContextExplorer } from "../ContextExplorer";
import { ScopeProvider } from "../ScopeProvider";
import {
  STALENESS_NOTE,
  freeScope,
  makeFreeSource,
  makePinnedSource,
  pinnedScope,
} from "./fixtures";

const meta: Meta<typeof ContextExplorer> = {
  title: "Context/ContextExplorer",
  component: ContextExplorer,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ContextExplorer>;

/** kapi's mount: one project on disk, the dimensions pinned to it. */
export const PinnedToOneProject: Story = {
  render: () => (
    <ScopeProvider
      source={makePinnedSource()}
      pinned={["workspace", "project", "stream"]}
      scope={pinnedScope}
    >
      <ContextExplorer />
    </ScopeProvider>
  ),
};

/** Bowrain's mount: a workspace, every dimension free. */
export const FreeAcrossAWorkspace: Story = {
  render: () => (
    <ScopeProvider source={makeFreeSource()} pinned={["workspace"]} scope={freeScope}>
      <ContextExplorer />
    </ScopeProvider>
  ),
};

/** The graph moved under the reader; every answer says so before anything else. */
export const BehindItsVenue: Story = {
  render: () => (
    <ScopeProvider
      source={makePinnedSource({ notes: [STALENESS_NOTE] })}
      pinned={["workspace", "project", "stream"]}
      scope={pinnedScope}
    >
      <ContextExplorer />
    </ScopeProvider>
  ),
};

/** A surface that reads governance only — the other panes state their reach. */
export const GovernanceOnly: Story = {
  render: () => (
    <ScopeProvider
      source={makePinnedSource({ withoutContent: true, withoutRelations: true })}
      pinned={["workspace", "project", "stream"]}
      scope={pinnedScope}
    >
      <ContextExplorer />
    </ScopeProvider>
  ),
};
