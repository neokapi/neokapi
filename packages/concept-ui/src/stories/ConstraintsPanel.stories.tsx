import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConstraintsPanel } from "../ConstraintsPanel";
import type { ConceptDataSource } from "../adapter";
import { PanelHarness, makePanelSource } from "./panel-fixtures";

const rich = makePanelSource();
const core = makePanelSource({ rich: false });
const failing: ConceptDataSource = {
  ...makePanelSource(),
  getRelations: () => Promise.reject(new Error("Server unavailable (503)")),
};

const meta: Meta<typeof ConstraintsPanel> = {
  title: "Concept UI/Panels/ConstraintsPanel",
  component: ConstraintsPanel,
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof ConstraintsPanel>;

/**
 * Several dated transitions across markets: ‘Kasse’ preferred in DACH since
 * Jan 2026 (open-ended), ‘Paiement’ preferred in France since Nov 2025,
 * ‘Validation de commande’ deprecated and now out of force, a closed seasonal
 * window, and a dated ‘use instead’ relation. The chart places each on one
 * scale with a today marker; the summary spells out banned-where vs
 * preferred-where.
 */
export const Rich: Story = {
  render: () => (
    <PanelHarness source={rich} conceptId="checkout" render={(p) => <ConstraintsPanel {...p} />} />
  ),
};

/**
 * The framework-only path: markets are derived from the term validity tags
 * rather than supplied. The same windows and the same banned/preferred summary
 * still read.
 */
export const CoreOnly: Story = {
  render: () => (
    <PanelHarness source={core} conceptId="checkout" render={(p) => <ConstraintsPanel {...p} />} />
  ),
};

/**
 * A flat termbase: no validity windows at all, so the time chart is skipped and
 * the panel falls back to the banned/preferred summary alone.
 */
export const SummaryOnly: Story = {
  render: () => (
    <PanelHarness source={core} conceptId="wishlist" render={(p) => <ConstraintsPanel {...p} />} />
  ),
};

/**
 * A failed fetch (the relations read rejects): the panel surfaces an error so a
 * load failure never reads as "no constraints".
 */
export const FetchError: Story = {
  render: () => (
    <PanelHarness
      source={failing}
      conceptId="checkout"
      render={(p) => <ConstraintsPanel {...p} />}
    />
  ),
};
