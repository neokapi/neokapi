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
import { PreviewRefusalError } from "../lib/previewPort";
import type { PreviewRefusal } from "../types/api";

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

/**
 * The Fernwell fixtures as a Bowrain server answers them: the member who kept
 * a rule is named, and the person signed in is shown by their name.
 */
const FERNWELL_SERVER: ProjectScreensPort = {
  ...FERNWELL_LOCAL,
  actorName: (actor) => actor.name || (actor.kind === "person" ? "Mira Holt" : undefined),
};

/** A port whose every read the server refuses. */
function refusing(refusal: PreviewRefusal): ProjectScreensPort {
  const refuse = async (): Promise<never> => {
    throw new PreviewRefusalError(refusal);
  };
  return { mapSummary: refuse, readerFile: refuse, decideView: refuse, keptRules: refuse };
}

const CONNECTED = {
  source: "bowrain" as const,
  serverURL: "https://app.bowrain.cloud",
  projectURL: "https://app.bowrain.cloud/acme/help",
};

const NO_PERMISSION = refusing({
  status: 403,
  message: "insufficient project permissions",
  action: "open",
});
const SIGNED_OUT = refusing({ status: 401, message: "invalid token", action: "sign_in" });

/** Connected and signed in: the server's own Rules, with its member names. */
export const ConnectedRules: Story = {
  args: { ...CONNECTED, port: FERNWELL_SERVER, defaultTab: "rules" },
  play: async () => {
    const sheet = await screen.findByRole("dialog");
    await expect(within(sheet).getByText("As Bowrain shows it")).toBeInTheDocument();
    const kept = await within(sheet).findByRole("region", { name: "Kept by" });
    await waitFor(() => expect(within(kept).getAllByText(/Mira Holt$/).length).toBeGreaterThan(0));
  },
};

export const ConnectedRulesDark: Story = {
  name: "Connected rules (dark)",
  args: { ...CONNECTED, port: FERNWELL_SERVER, defaultTab: "rules" },
  globals: { theme: "dark" },
};

/** The server refuses the project: its words, and Open Bowrain. */
export const ConnectedNoPermission: Story = {
  args: { ...CONNECTED, port: NO_PERMISSION },
  play: async () => {
    const sheet = await screen.findByRole("dialog");
    const body = await within(sheet).findByRole("alert");
    await expect(body).toHaveTextContent("insufficient project permissions");
    await expect(within(body).getByRole("link", { name: /Open Bowrain/ })).toBeInTheDocument();
  },
};

/** The server does not take the sign-in: its words, and the command that signs in. */
export const ConnectedSignedOut: Story = {
  args: { ...CONNECTED, port: SIGNED_OUT },
  play: async () => {
    const sheet = await screen.findByRole("dialog");
    const body = await within(sheet).findByRole("alert");
    await expect(body).toHaveTextContent("kapi auth login --server https://app.bowrain.cloud");
  },
};

export const ConnectedSignedOutDark: Story = {
  name: "Connected signed out (dark)",
  args: { ...CONNECTED, port: SIGNED_OUT },
  globals: { theme: "dark" },
};
