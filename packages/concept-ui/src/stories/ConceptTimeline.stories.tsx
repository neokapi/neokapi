import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConceptTimeline } from "../ConceptTimeline";
import type { ConceptDataSource } from "../adapter";
import { PanelHarness, makePanelSource } from "./panel-fixtures";

const rich = makePanelSource();
const core = makePanelSource({ rich: false });
const failing: ConceptDataSource = {
  ...makePanelSource(),
  getRelations: () => Promise.reject(new Error("Server unavailable (503)")),
};

const meta: Meta<typeof ConceptTimeline> = {
  title: "Concept UI/Panels/ConceptTimeline",
  component: ConceptTimeline,
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof ConceptTimeline>;

/**
 * The platform path: the full revision log — created, revised, status changes,
 * a relation, a competitor observation, a discussion reply, and a change-set —
 * grouped by day, each event iconed by kind. Two events fall on the same day to
 * show in-day grouping; use the Newest/Oldest toggle to flip direction.
 */
export const Rich: Story = {
  render: () => (
    <PanelHarness source={rich} conceptId="checkout" render={(p) => <ConceptTimeline {...p} />} />
  ),
};

/**
 * The framework-only path (a local SQLite termbase): no revision log, so the
 * timeline is synthesised from what the termbase knows — create/update plus the
 * status and relation changes inferred from validity windows. A caption marks
 * the degradation.
 */
export const CoreOnly: Story = {
  render: () => (
    <PanelHarness source={core} conceptId="checkout" render={(p) => <ConceptTimeline {...p} />} />
  ),
};

/** The floor case: an undated concept has no history to show. */
export const Empty: Story = {
  render: () => (
    <PanelHarness source={core} conceptId="wishlist" render={(p) => <ConceptTimeline {...p} />} />
  ),
};

/**
 * A failed fetch (the relations read rejects): the panel surfaces an error so a
 * load failure never reads as "no history".
 */
export const FetchError: Story = {
  render: () => (
    <PanelHarness
      source={failing}
      conceptId="checkout"
      render={(p) => <ConceptTimeline {...p} />}
    />
  ),
};
