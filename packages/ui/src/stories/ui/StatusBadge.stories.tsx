import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  CONTENT_STATUS_LADDER,
  SOURCE_STATUS_LADDER,
  StatusBadge,
} from "../../components/ui/status-badge";

/**
 * Two ladders on one scale: a target climbs draft to signed-off, a source
 * climbs authored to approved, and the rungs that mean the same thing are the
 * same colour.
 */
const meta: Meta<typeof StatusBadge> = {
  title: "Foundations/StatusBadge",
  component: StatusBadge,
  parameters: { layout: "padded" },
  args: { ladder: "content", status: "reviewed" },
};
export default meta;

type Story = StoryObj<typeof StatusBadge>;

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

function Ladders() {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <span className="w-16 text-xs text-muted-foreground">content</span>
        <StatusBadge ladder="content" status="not-started" />
        {CONTENT_STATUS_LADDER.map((s) => (
          <StatusBadge key={s} ladder="content" status={s} />
        ))}
      </div>
      <div className="flex items-center gap-2">
        <span className="w-16 text-xs text-muted-foreground">source</span>
        {SOURCE_STATUS_LADDER.map((s) => (
          <StatusBadge key={s} ladder="source" status={s} />
        ))}
      </div>
      <div className="flex items-center gap-2">
        <span className="w-16 text-xs text-muted-foreground">held</span>
        <StatusBadge ladder="content" status="blocked" />
        <StatusBadge ladder="source" status="attention" />
      </div>
    </div>
  );
}

/** Both ladders in both themes, so the two scales can be read against each other. */
export const Ladder: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      <Panel>
        <Ladders />
      </Panel>
      <Panel dark>
        <Ladders />
      </Panel>
    </div>
  ),
};

/** The dense form, for a table cell or a coverage grid. */
export const Compact: Story = {
  render: () => (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-1.5">
        {CONTENT_STATUS_LADDER.map((s) => (
          <StatusBadge key={s} ladder="content" status={s} compact />
        ))}
      </div>
      <div className="flex items-center gap-1.5">
        {SOURCE_STATUS_LADDER.map((s) => (
          <StatusBadge key={s} ladder="source" status={s} compact />
        ))}
      </div>
    </div>
  ),
};

/** A rung the UI has not styled keeps its own text rather than disappearing. */
export const UnknownStatus: Story = {
  render: () => (
    <div className="flex items-center gap-1.5">
      <StatusBadge ladder="content" status="proofread" />
      <StatusBadge ladder="source" status="drafting" />
    </div>
  ),
};
