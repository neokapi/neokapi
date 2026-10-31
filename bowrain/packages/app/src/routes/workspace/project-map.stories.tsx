import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { ProjectMapPage } from "./project-map";
import { fixtureScreensPort } from "./project-screens-fixture-port";
import { permittedPort } from "./project-screens-port";

/**
 * The Map route, the page a Bowrain project opens on, drawn over the Fernwell
 * fixtures. A member who may view the content runs the checks over the whole
 * project.
 */
const meta: Meta<typeof ProjectMapPage> = {
  title: "Bowrain/Project/Map",
  component: ProjectMapPage,
  args: { projectName: "Fernwell", navigate: fn(), port: fixtureScreensPort() },
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof ProjectMapPage>;

/** A member with every permission. */
export const Member: Story = {};

/** A member who may not run the checks: the page draws no check action. */
export const WithoutPermission: Story = {
  args: { port: permittedPort(fixtureScreensPort(), () => false) },
};
