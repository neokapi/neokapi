import type { Meta, StoryObj } from "@storybook/react-vite";
import { Check, RotateCcw, Trash2 } from "lucide-react";
import { Button } from "../../components/ui/button";

/**
 * The judgement a button carries is its colour, and the colour means the same
 * thing on every surface. See packages/ui/docs/judgement-colours.md.
 */
const meta: Meta<typeof Button> = {
  title: "Foundations/Semantic Buttons",
  component: Button,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Button>;

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

function Row() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="default">Run</Button>
      <Button variant="success">
        <Check data-icon="inline-start" />
        Approve
      </Button>
      <Button variant="warning">
        <RotateCcw data-icon="inline-start" />
        Send back
      </Button>
      <Button variant="destructive">
        <Trash2 data-icon="inline-start" />
        Reject
      </Button>
      <Button variant="secondary">Cancel</Button>
      <Button variant="ghost">Skip</Button>
    </div>
  );
}

/** Every judgement variant beside the primary and the quiet ones. */
export const Judgements: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      <Panel>
        <Row />
      </Panel>
      <Panel dark>
        <Row />
      </Panel>
    </div>
  ),
};

/** The new variants across the size scale, so a toolbar and a form agree. */
export const Sizes: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      {(["xs", "sm", "default", "lg"] as const).map((size) => (
        <div key={size} className="flex items-center gap-2">
          <span className="w-16 text-xs text-muted-foreground">{size}</span>
          <Button variant="success" size={size}>
            Sign off
          </Button>
          <Button variant="warning" size={size}>
            Needs attention
          </Button>
          <Button variant="success" size={size} disabled>
            Sign off
          </Button>
        </div>
      ))}
    </div>
  ),
};
