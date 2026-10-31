import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import type { ProjectScreensPort } from "@neokapi/project-screens";
import {
  DECIDE_VIEW,
  KEPT_RULES,
  MAP_PENDING,
  READER_WITH_FIXES,
} from "@neokapi/project-screens/fixtures";
import { BowrainPreviewSheet } from "../components/BowrainPreviewSheet";

/** The Fernwell fixtures through the four reads a local preview makes. */
const FERNWELL_LOCAL: ProjectScreensPort = {
  mapSummary: async () => MAP_PENDING,
  readerFile: async () => READER_WITH_FIXES,
  decideView: async () => DECIDE_VIEW,
  keptRules: async () => KEPT_RULES,
};

/**
 * Preview on the Share page's Bowrain rung: the project's Map and Rules as a
 * Bowrain project page draws them, read from this Mac, with no Keep, Fix or
 * check action.
 */
const meta: Meta<typeof BowrainPreviewSheet> = {
  title: "Components/BowrainPreviewSheet",
  component: BowrainPreviewSheet,
  args: {
    open: true,
    onOpenChange: fn(),
    projectName: "Fernwell",
    port: FERNWELL_LOCAL,
  },
};

export default meta;
type Story = StoryObj<typeof BowrainPreviewSheet>;

/** The Map tab: the counts, the suggestions and the files. */
export const MapTab: Story = {
  play: async () => {
    const sheet = await screen.findByRole("dialog");
    await within(sheet).findByRole("heading", { name: "Fernwell" });
    await expect(within(sheet).queryByRole("button", { name: "Run on everything" })).toBeNull();
  },
};

export const MapTabDark: Story = { name: "Map tab (dark)", globals: { theme: "dark" } };

/** The Rules tab: the questions waiting, then Kept by with who kept each rule. */
export const RulesTab: Story = {
  args: { defaultTab: "rules" },
  play: async () => {
    const sheet = await screen.findByRole("dialog");
    await within(sheet).findByRole("region", { name: "Kept by" });
    await expect(within(sheet).queryByRole("button", { name: /^Keep/ })).toBeNull();
  },
};

export const RulesTabDark: Story = {
  name: "Rules tab (dark)",
  args: { defaultTab: "rules" },
  globals: { theme: "dark" },
};

/** A file on the Map opened in the Reader inside the sheet, with Back to the Map. */
export const ReaderInside: Story = {
  play: async () => {
    const sheet = await screen.findByRole("dialog");
    await within(sheet).findByRole("heading", { name: "Fernwell" });
    await userEvent.click(within(sheet).getAllByRole("button", { name: "Open" })[0]);
    await waitFor(() =>
      expect(
        within(sheet).getByRole("heading", { name: READER_WITH_FIXES.path }),
      ).toBeInTheDocument(),
    );
    await expect(within(sheet).queryByRole("button", { name: /^Fix/ })).toBeNull();
  },
};
