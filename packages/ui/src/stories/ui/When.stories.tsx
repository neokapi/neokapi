import type { Meta, StoryObj } from "@storybook/react-vite";
import { When } from "../../components/ui/when";

/**
 * One rendering for an instant: the date and time in the reader's own language,
 * the exact instant with its zone in the tooltip, and the ISO string in the
 * element's `dateTime` for anything reading the page as data.
 */
const meta: Meta<typeof When> = {
  title: "Foundations/When",
  component: When,
  parameters: { layout: "padded" },
  args: { iso: "2026-08-30T09:12:00Z" },
};
export default meta;

type Story = StoryObj<typeof When>;

const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

function ago(ms: number): string {
  return new Date(Date.now() - ms).toISOString();
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-baseline gap-6 text-sm">
      <span className="w-40 shrink-0 text-xs text-muted-foreground">{label}</span>
      {children}
    </div>
  );
}

/** The default: a medium date and a short time, with the exact instant on hover. */
export const DateAndTime: Story = {
  render: (args) => (
    <div className="flex flex-col gap-2">
      <Row label="Default">
        <When {...args} />
      </Row>
      <Row label="Date only">
        <When {...args} timeStyle="none" />
      </Row>
      <Row label="Time only">
        <When {...args} dateStyle="none" />
      </Row>
      <Row label="Full date, long time">
        <When {...args} dateStyle="full" timeStyle="long" />
      </Row>
    </div>
  ),
};

/** `relative` answers "how long ago", down to "now" and up to years. */
export const Relative: Story = {
  render: () => (
    <div className="flex flex-col gap-2">
      <Row label="Inside the minute">
        <When iso={ago(20_000)} relative />
      </Row>
      <Row label="Minutes">
        <When iso={ago(3 * MINUTE)} relative />
      </Row>
      <Row label="Hours">
        <When iso={ago(5 * HOUR)} relative />
      </Row>
      <Row label="Yesterday">
        <When iso={ago(DAY)} relative />
      </Row>
      <Row label="Weeks">
        <When iso={ago(18 * DAY)} relative />
      </Row>
      <Row label="Months">
        <When iso={ago(120 * DAY)} relative />
      </Row>
    </div>
  ),
};

/** The instant follows the reader. One moment, named in three languages. */
export const InAnotherUILanguage: Story = {
  render: (args) => (
    <div className="flex flex-col gap-2">
      {["en", "fr", "nb", "ja", "ar"].map((tag) => (
        <Row key={tag} label={tag}>
          <When {...args} uiLocale={tag} />
        </Row>
      ))}
    </div>
  ),
};

/** A value the runtime reads as no date is returned as it was given. */
export const NotADate: Story = {
  render: () => (
    <div className="flex flex-col gap-2">
      <Row label="Unparseable">
        <When iso="whenever" />
      </Row>
      <Row label="Empty">
        <When iso="" />
      </Row>
    </div>
  ),
};
