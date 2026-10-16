import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StatRow } from "../../components/ui/stat-row";
import type { StatItem } from "../../components/ui/stat-row";

/**
 * A row of labelled counts, the second region of an overview page. Each
 * number is a count with a label rather than a clause in a sentence, and the
 * one count that waits for a person takes the warning ink.
 */
const meta: Meta<typeof StatRow> = {
  title: "Patterns/StatRow",
  component: StatRow,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof StatRow>;

const OVERVIEW: StatItem[] = [
  { value: 4, label: "projects" },
  { value: 61, label: "rules" },
  { value: 11, label: "suggestions" },
  { value: 1, label: "needs you", tone: "warning" },
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

/** The overview's four counts. */
export const Default: Story = {
  args: { items: OVERVIEW.map((item) => ({ ...item, tone: undefined })) },
};

/** One count needs a person, in both themes. A zero stays in the row as 0. */
export const WithAWarningCount: Story = {
  name: "With a warning count",
  render: () => (
    <div className="flex flex-col gap-4">
      {[false, true].map((dark) => (
        <Panel key={String(dark)} dark={dark}>
          <div className="flex flex-col gap-5">
            <StatRow items={OVERVIEW} />
            <StatRow
              items={[
                { value: 4, label: "projects" },
                { value: 61, label: "rules" },
                { value: 0, label: "suggestions", tone: "muted" },
                { value: 0, label: "need you", tone: "warning" },
              ]}
            />
          </div>
        </Panel>
      ))}
    </div>
  ),
};

/** Many counts in a narrow pane wrap onto a second line rather than squeezing. */
export const ManyOnANarrowPane: Story = {
  name: "Many on a narrow pane",
  render: () => (
    <div className="w-[300px] rounded-lg border p-4">
      <StatRow
        items={[
          { value: 12, label: "projects" },
          { value: 1204, label: "rules" },
          { value: 87, label: "suggestions" },
          { value: 3, label: "need you", tone: "warning" },
          { value: 412, label: "terms" },
          { value: 9, label: "voice profiles" },
          { value: 26, label: "new this week", tone: "muted" },
        ]}
      />
    </div>
  ),
};

/** Counts that lead somewhere are buttons; a hint says more on hover or focus. */
export const Clickable: Story = {
  render: function ClickableStory() {
    const [opened, setOpened] = useState<string | null>(null);
    return (
      <div className="flex flex-col gap-4">
        <StatRow
          items={[
            { value: 4, label: "projects", onClick: () => setOpened("projects") },
            { value: 61, label: "rules", onClick: () => setOpened("rules") },
            {
              value: 11,
              label: "suggestions",
              hint: "Recorded by your assistants and not yet kept",
              onClick: () => setOpened("suggestions"),
            },
            {
              value: 1,
              label: "needs you",
              tone: "warning",
              hint: "Two assistants disagree",
              onClick: () => setOpened("needs you"),
            },
          ]}
        />
        <p className="text-[11px] text-muted-foreground">
          {opened ? `Opened ${opened}` : "Choose a count"}
        </p>
      </div>
    );
  },
};
