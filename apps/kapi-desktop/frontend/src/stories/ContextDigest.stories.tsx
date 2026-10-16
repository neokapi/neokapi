import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { ContextDigestView } from "../components/ContextDigest";
import { ContextWidenDialog, widenRuleOfDigestItem } from "../components/ContextWidenDialog";
import {
  CONTEXT_DIGEST,
  EMPTY_DIGEST,
  LAST_LOOKED,
  QUIET_DIGEST,
  SAMPLE_DIGEST,
  SAMPLE_IDS,
  manySuggestions,
  onlySection,
  studioRule,
} from "./fixtures/contextDigest";

/**
 * What's new for a project, as a list and a detail pane: Needs you,
 * Suggested, Established, Earlier and Drift on the left, one line each, and
 * the selected row's evidence and decisions on the right.
 */
const meta: Meta<typeof ContextDigestView> = {
  title: "Components/ContextDigest",
  component: ContextDigestView,
  tags: ["autodocs"],
  args: {
    digest: SAMPLE_DIGEST,
    since: LAST_LOOKED,
    keyboard: false,
    onKeep: fn(),
    onKeepGroup: fn(),
    onDrop: fn(),
    onChoose: fn(),
    onWiden: fn(),
    onOpenFile: fn(),
  },
  decorators: [
    (Story) => (
      <div className="h-[560px] overflow-hidden rounded-lg border border-border bg-background">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ContextDigestView>;

const dark = { globals: { theme: "dark" } };

/** The KapiMart sample on first open: one disagreement, selected, and five suggestions. */
export const Sample: Story = {};

export const SampleDark: Story = { ...dark, name: "Sample (dark)" };

/** A suggestion selected: Keep, Change and Drop in the toolbar, the evidence and the facts below. */
export const SuggestionSelected: Story = {
  args: { defaultSelected: `suggested:${SAMPLE_IDS.kapiMart}` },
};

export const SuggestionSelectedDark: Story = {
  ...dark,
  name: "Suggestion selected (dark)",
  args: { defaultSelected: `suggested:${SAMPLE_IDS.kapiMart}` },
};

/** A disagreement selected: its two sides beside each other, each with its own Choose this side. */
export const ConflictSelected: Story = {
  args: {
    digest: CONTEXT_DIGEST,
    defaultSelected: `conflict:${CONTEXT_DIGEST.conflicts[0].sides[0].id}`,
  },
};

export const ConflictSelectedDark: Story = {
  ...dark,
  name: "Conflict selected (dark)",
  args: { digest: CONTEXT_DIGEST },
};

/** A rule in force selected: Apply more widely and Drop, and how it was established. */
export const EstablishedSelected: Story = {
  args: { digest: CONTEXT_DIGEST, defaultSelected: `established:${studioRule().id}` },
};

export const EstablishedSelectedDark: Story = {
  ...dark,
  name: "Established selected (dark)",
  args: { digest: CONTEXT_DIGEST, defaultSelected: `established:${studioRule().id}` },
};

/** Change: the field takes the toolbar's place, holding the form the suggestion asks for. */
export const ChangeFormOpen: Story = {
  args: { defaultSelected: `suggested:${SAMPLE_IDS.cart}` },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Change" }));
    const field = await canvas.findByRole("textbox", { name: "What to write instead" });
    await expect(field).toHaveValue("cart");
  },
};

export const ChangeFormOpenDark: Story = {
  ...ChangeFormOpen,
  ...dark,
  name: "Change form open (dark)",
};

/** Content moving away from a rule in force, with the project's own words back to it. */
export const Drift: Story = {
  args: {
    digest: onlySection("drift"),
    defaultSelected: `drift:${CONTEXT_DIGEST.drift[0].rule.id}`,
  },
};

export const DriftDark: Story = { ...Drift, ...dark, name: "Drift (dark)" };

/** Forty suggestions: each theme takes a sticky header of its own as the list scrolls. */
export const ManyItems: Story = { args: { digest: manySuggestions(40) } };

export const ManyItemsDark: Story = { ...ManyItems, ...dark, name: "Many items (dark)" };

/** Nothing new since the last look: one muted line above the list, and the rules from before under Earlier. */
export const NothingNew: Story = { args: { digest: QUIET_DIGEST } };

/** A project nobody has recorded anything about: what would appear here, and Connect an assistant. */
export const Empty: Story = {
  args: { digest: EMPTY_DIGEST, since: undefined, onConnectAssistant: fn() },
};

export const EmptyDark: Story = { ...Empty, ...dark, name: "Empty (dark)" };

/** No checkout of the project on this machine: the digest reads, and offers no decisions. */
export const ReadOnly: Story = {
  args: {
    canDecide: false,
    onOpenFile: undefined,
    defaultSelected: `suggested:${SAMPLE_IDS.kapiMart}`,
  },
};

/**
 * The keyboard session: j moves the selection, a keeps the selected
 * suggestion and the selection moves on to the next one.
 */
export const Keyboard: Story = {
  args: { keyboard: true, defaultSelected: `suggested:${SAMPLE_IDS.kapiMart}` },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);
    const selected = () =>
      canvasElement.querySelector('[data-slot="digest-item"][aria-selected="true"]');
    await expect(selected()).toHaveAttribute("data-id", SAMPLE_IDS.kapiMart);
    await userEvent.keyboard("a");
    await expect(args.onKeep).toHaveBeenCalledWith(
      expect.objectContaining({ id: SAMPLE_IDS.kapiMart }),
      undefined,
    );
    await waitFor(() => expect(selected()).toHaveAttribute("data-id", SAMPLE_IDS.cart));
    await userEvent.keyboard("k");
    await expect(selected()).toHaveAttribute("data-id", SAMPLE_IDS.kapiMart);
    await expect(canvas.getByRole("heading", { level: 2 })).toHaveTextContent("Write KapiMart");
  },
};

/** The preview "Apply more widely" opens before the rule answers in every project. */
export const ApplyMoreWidelyPreview: StoryObj<typeof ContextWidenDialog> = {
  render: () => (
    <ContextWidenDialog
      rule={widenRuleOfDigestItem("fernwell", studioRule())}
      to="workspace"
      onClose={fn()}
      onConfirm={fn()}
      preview={{
        to: "workspace",
        from: { level: "project", describe: "project product=studio" },
        scope: { level: "workspace", describe: "workspace product=studio" },
        rule: { kind: "term", term: "business", replacement: "studio" },
        projects: [
          { project_key: "fernwell-site", project_name: "Fernwell site", checked_out: true },
        ],
        points: [],
        units: [
          {
            project_key: "fernwell-site",
            document: "pages/about.md",
            unit: "p2",
            text: "The business opened its doors in 2019.",
            matches: 1,
          },
        ],
        coverage: {
          examined: [
            { project_key: "fernwell", project_name: "Fernwell", units: 48, matched: 0 },
            { project_key: "fernwell-site", project_name: "Fernwell site", units: 21, matched: 1 },
          ],
          not_examined: [],
          truncated: false,
        },
      }}
    />
  ),
};
