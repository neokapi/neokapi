import type { Meta, StoryObj } from "@storybook/react-vite";
import { AXIS_IDS, CoordinateChip } from "../../components/ui/coordinate-chip";

/**
 * A coordinate is an address: product, channel, brand, language. Each axis gets
 * a hue and an icon, and its name is spelt out in the tooltip and the
 * accessible name so nobody has to learn the colours.
 */
const meta: Meta<typeof CoordinateChip> = {
  title: "Foundations/CoordinateChip",
  component: CoordinateChip,
  parameters: { layout: "padded" },
  args: { axis: "channel", value: "reference" },
};
export default meta;

type Story = StoryObj<typeof CoordinateChip>;

const SAMPLE: Array<{ axis: string; value: string }> = [
  { axis: "product", value: "kapi" },
  { axis: "channel", value: "reference" },
  { axis: "brand", value: "Bowrain" },
  { axis: "language", value: "fr-FR" },
  { axis: "region", value: "EMEA" },
];

function Panel({ dark, children }: { dark?: boolean; children: React.ReactNode }) {
  return (
    <div className={dark ? "dark" : undefined}>
      <div className="rounded-lg border bg-background p-4 text-foreground">
        <p className="mb-3 text-xs font-medium text-muted-foreground">{dark ? "Dark" : "Light"}</p>
        {children}
      </div>
    </div>
  );
}

/** Every axis, plus one a recipe invented, in both themes. */
export const Axes: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      {[false, true].map((dark) => (
        <Panel key={String(dark)} dark={dark}>
          <div className="flex flex-wrap items-center gap-1.5">
            {SAMPLE.map((c) => (
              <CoordinateChip key={c.axis} axis={c.axis} value={c.value} />
            ))}
          </div>
        </Panel>
      ))}
    </div>
  ),
};

/** A whole point, the way a collection row shows where its content sits. */
export const APoint: Story = {
  name: "A point",
  render: () => (
    <div className="flex items-center gap-2 rounded-lg border p-3">
      <span className="text-sm font-medium">docs/reference</span>
      <span className="flex flex-wrap gap-1.5">
        {AXIS_IDS.map((axis) => (
          <CoordinateChip
            key={axis}
            axis={axis}
            value={
              { product: "kapi", channel: "reference", brand: "Bowrain", language: "nb-NO" }[axis]
            }
          />
        ))}
      </span>
    </div>
  ),
};

/** The larger size, and the removable form a filter bar uses. */
export const SizesAndRemoval: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <span className="w-16 text-xs text-muted-foreground">sm</span>
        <CoordinateChip axis="channel" value="reference" />
        <CoordinateChip axis="brand" value="Bowrain" />
      </div>
      <div className="flex items-center gap-2">
        <span className="w-16 text-xs text-muted-foreground">md</span>
        <CoordinateChip axis="channel" value="reference" size="md" />
        <CoordinateChip axis="brand" value="Bowrain" size="md" />
      </div>
      <div className="flex items-center gap-2">
        <span className="w-16 text-xs text-muted-foreground">remove</span>
        <CoordinateChip axis="product" value="kapi" onRemove={() => {}} />
        <CoordinateChip axis="language" value="pt-BR" size="md" onRemove={() => {}} />
      </div>
    </div>
  ),
};

/** Casing survives: a value is an identifier, not a sentence. */
export const CasingIsKept: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-1.5">
      <CoordinateChip axis="brand" value="Bowrain" />
      <CoordinateChip axis="brand" value="bowrain-hq" />
      <CoordinateChip axis="language" value="zh-Hant" />
      <CoordinateChip axis="language" value="sr-Latn-RS" />
    </div>
  ),
};
