import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { DECIDE_IDS } from "@neokapi/project-screens/fixtures";
import { ProjectRulesPage } from "./project-rules";
import { fixtureScreensPort } from "./project-screens-fixture-port";
import { permittedPort } from "./project-screens-port";

/**
 * The Rules route over the Fernwell fixtures: the questions about the
 * project's suggestions, and the rules in force with who kept each.
 */
const meta: Meta<typeof ProjectRulesPage> = {
  title: "Bowrain/Project/Rules",
  component: ProjectRulesPage,
  args: { navigate: fn(), port: fixtureScreensPort() },
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof ProjectRulesPage>;

/** A member who governs rules. */
export const Member: Story = {};

/** Opened on one question from a Map row. */
export const OnARow: Story = { args: { row: `suggested:${DECIDE_IDS.quickcast}` } };

/** A member who governs no rule: Decide draws no answer. */
export const WithoutPermission: Story = {
  args: { port: permittedPort(fixtureScreensPort(), (p) => p === "view_content") },
};
