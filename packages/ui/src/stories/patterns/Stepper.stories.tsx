import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Stepper, type StepState, type StepperStep } from "../../components/ui/stepper";

/**
 * A ladder of steps: done, current, or still to do. The connector up to the
 * current step takes the primary colour. Each step is a button, and the
 * selected one is the step whose detail the page shows, so a reader can click
 * ahead and read about a step the subject has not reached. Under 480 pixels
 * the steps stack with the connector down the start edge.
 */
const meta: Meta<typeof Stepper> = {
  title: "Patterns/Stepper",
  component: Stepper,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Stepper>;

const NAMES = ["On this machine", "Shared with your team", "Managed on a server"];

function ladder(states: StepState[]): StepperStep[] {
  return states.map((state, i) => ({ id: `step-${i + 1}`, label: NAMES[i], state }));
}

/** The combinations a three-rung ladder takes, the last with a rung skipped. */
const COMBINATIONS: Array<{ name: string; states: StepState[] }> = [
  { name: "First step current", states: ["current", "todo", "todo"] },
  { name: "Second step current", states: ["done", "current", "todo"] },
  { name: "Last step current", states: ["done", "done", "current"] },
  { name: "Last step current, second skipped", states: ["done", "todo", "current"] },
  { name: "Every step to do", states: ["todo", "todo", "todo"] },
  { name: "Every step done", states: ["done", "done", "done"] },
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

/** One ladder that a click or an arrow key moves the selection on. */
function Live({ states, initial }: { states: StepState[]; initial?: string }) {
  const steps = ladder(states);
  const [value, setValue] = useState(initial);
  return (
    <Stepper
      aria-label="How far this context reaches"
      steps={steps}
      value={value}
      onChange={setValue}
    />
  );
}

function Combinations() {
  return (
    <div className="flex flex-col gap-6">
      {COMBINATIONS.map(({ name, states }) => (
        <div key={name}>
          <p className="mb-2 text-[11px] text-muted-foreground">{name}</p>
          <Live states={states} />
        </div>
      ))}
    </div>
  );
}

/** Every combination of states, in both themes. */
export const States: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      <Panel>
        <Combinations />
      </Panel>
      <Panel dark>
        <Combinations />
      </Panel>
    </div>
  ),
};

/** The reader has clicked ahead: the third step is selected while the first is current. */
export const ClickedAhead: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      <Panel>
        <Live states={["current", "todo", "todo"]} initial="step-3" />
      </Panel>
      <Panel dark>
        <Live states={["current", "todo", "todo"]} initial="step-3" />
      </Panel>
    </div>
  ),
};

/** Narrower than 480 pixels, the steps stack with the connector down the start edge. */
export const Stacked: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      {[false, true].map((dark) => (
        <Panel key={String(dark)} dark={dark}>
          <div className="flex flex-wrap gap-8">
            {COMBINATIONS.slice(0, 3).map(({ name, states }) => (
              <div key={name} className="w-64">
                <p className="mb-2 text-[11px] text-muted-foreground">{name}</p>
                <Live states={states} />
              </div>
            ))}
          </div>
        </Panel>
      ))}
    </div>
  ),
};

/** A long name wraps under its dot rather than pushing the next step aside. */
export const LongLabels: Story = {
  render: () => (
    <Panel>
      <Stepper
        aria-label="Release"
        steps={[
          { id: "draft", label: "Drafted by the assistant", state: "done" },
          {
            id: "review",
            label: "Reviewed by a person who owns the voice profile",
            state: "current",
          },
          { id: "publish", label: "Published", state: "todo" },
        ]}
      />
    </Panel>
  ),
};
