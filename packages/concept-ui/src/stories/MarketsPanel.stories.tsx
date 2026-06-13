import { useMemo } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { MarketsPanel } from "../MarketsPanel";
import type { ConceptDataSource } from "../adapter";
import { resolveCapabilities } from "../adapter";
import { useResource } from "../useResource";
import { makeMemorySource } from "./fixtures";

function MarketsHarness({
  source,
  conceptId = "checkout",
}: {
  source: ConceptDataSource;
  conceptId?: string;
}) {
  const caps = useMemo(() => resolveCapabilities(source), [source]);
  const { data: concept } = useResource(() => source.getConcept(conceptId), [source, conceptId]);
  return (
    <div className="mx-auto max-w-2xl p-6">
      {concept && (
        <MarketsPanel
          concept={concept}
          source={source}
          capabilities={caps}
          onNavigate={() => undefined}
        />
      )}
    </div>
  );
}

const richSource = makeMemorySource();
const coreSource = makeMemorySource({ rich: false, editable: false });
const failingSource: ConceptDataSource = {
  ...makeMemorySource(),
  getMarkets: () => Promise.reject(new Error("Server unavailable (503)")),
};

const meta: Meta<typeof MarketsPanel> = {
  title: "Concept UI/MarketsPanel",
  component: MarketsPanel,
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof MarketsPanel>;

/**
 * Named markets (DACH, France, US, UK): one panel each, with the term and status
 * used there. The French panel carries a deprecated variant, so its accent reads
 * banned; preferred wording is starred.
 */
export const NamedMarkets: Story = {
  render: () => <MarketsHarness source={richSource} />,
};

/**
 * A concept with a forbidden competitor term ('Voucher' in en-US): the panel
 * covering that locale turns its accent destructive and strikes the banned term.
 */
export const BannedWording: Story = {
  render: () => <MarketsHarness source={richSource} conceptId="coupon" />,
};

/**
 * Framework-only mode: no named markets, so the panels are derived from the
 * terms' `market` validity tags, with untagged locales gathered under "Other
 * locales". The header shows a "from tags" badge.
 */
export const FrameworkOnly: Story = {
  render: () => <MarketsHarness source={coreSource} />,
};

/**
 * The named-markets read fails: the panel surfaces an error instead of silently
 * degrading to tag-derived markets, so a failed fetch is never mistaken for a
 * concept with no regional wording.
 */
export const FetchError: Story = {
  render: () => <MarketsHarness source={failingSource} />,
};
