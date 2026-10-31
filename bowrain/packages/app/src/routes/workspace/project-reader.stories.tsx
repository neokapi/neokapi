import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectReaderPage } from "./project-reader";
import { fixtureScreensPort } from "./project-screens-fixture-port";
import { permittedPort } from "./project-screens-port";

/**
 * The Reader route on one Fernwell item. Fix files a source proposal, and the
 * finding's row then shows the proposal's id.
 */
const meta: Meta<typeof ProjectReaderPage> = {
  title: "Bowrain/Project/Reader",
  component: ProjectReaderPage,
  args: {
    projectId: "p1",
    path: "docs/billing.md",
    navigate: fn(),
    port: fixtureScreensPort(),
  },
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof ProjectReaderPage>;

/** A member who may file a proposal. */
export const Member: Story = {};

/** Opened at one block from a Rules evidence link. */
export const AtBlock: Story = { args: { block: "p3" } };

/** A member who may not file a proposal: the page draws no Fix. */
export const WithoutPermission: Story = {
  args: { port: permittedPort(fixtureScreensPort(), (p) => p === "view_content") },
};
