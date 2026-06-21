import type { Meta, StoryObj } from "@storybook/react-vite";
import { PipelineProgress } from "../components/PipelineProgress";
import type { StepSnapshot } from "../context/JobFeedContext";

const meta: Meta<typeof PipelineProgress> = {
  title: "Components/Pipeline Progress",
  component: PipelineProgress,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Visualizes a streaming pipeline's step-by-step progress using Badge-based step indicators. Each step transitions through pending, active (with spinner and part counts), and done states based on real-time atomic counter snapshots from the backend.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof PipelineProgress>;

const twoSteps = [{ tool: "translate" }, { tool: "qa" }];

const fourSteps = [
  { tool: "recycle" },
  { tool: "translate" },
  { tool: "qa" },
  { tool: "term-enforce" },
];

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

export const Idle: Story = {
  name: "Idle (all pending)",
  args: {
    steps: twoSteps,
    runState: "idle",
  },
};

export const AllPending: Story = {
  name: "Running — all pending",
  args: {
    steps: fourSteps,
    runState: "running",
    snapshots: [],
  },
};

export const FirstStepActive: Story = {
  name: "Running — first step active",
  args: {
    steps: fourSteps,
    runState: "running",
    snapshots: [
      { name: "recycle", parts_in: 47, parts_out: 32 },
      { name: "translate", parts_in: 0, parts_out: 0 },
      { name: "qa", parts_in: 0, parts_out: 0 },
      { name: "term-enforce", parts_in: 0, parts_out: 0 },
    ],
  },
};

export const MidPipeline: Story = {
  name: "Running — mid-pipeline",
  args: {
    steps: fourSteps,
    runState: "running",
    snapshots: [
      { name: "recycle", parts_in: 120, parts_out: 120 },
      { name: "translate", parts_in: 120, parts_out: 87 },
      { name: "qa", parts_in: 87, parts_out: 52 },
      { name: "term-enforce", parts_in: 0, parts_out: 0 },
    ],
  },
};

export const NearComplete: Story = {
  name: "Running — near complete",
  args: {
    steps: twoSteps,
    runState: "running",
    snapshots: [
      { name: "translate", parts_in: 120, parts_out: 120 },
      { name: "qa", parts_in: 120, parts_out: 118 },
    ],
  },
};

export const Complete: Story = {
  name: "Complete",
  args: {
    steps: fourSteps,
    runState: "complete",
    snapshots: [
      { name: "recycle", parts_in: 120, parts_out: 120 },
      { name: "translate", parts_in: 120, parts_out: 120 },
      { name: "qa", parts_in: 120, parts_out: 120 },
      { name: "term-enforce", parts_in: 120, parts_out: 120 },
    ],
  },
};

export const Error: Story = {
  name: "Error (frozen mid-run)",
  args: {
    steps: fourSteps,
    runState: "error",
    snapshots: [
      { name: "recycle", parts_in: 120, parts_out: 120 },
      { name: "translate", parts_in: 45, parts_out: 12 },
      { name: "qa", parts_in: 0, parts_out: 0 },
      { name: "term-enforce", parts_in: 0, parts_out: 0 },
    ],
  },
};

export const Canceled: Story = {
  name: "Canceled",
  args: {
    steps: twoSteps,
    runState: "canceled",
    snapshots: [
      { name: "translate", parts_in: 60, parts_out: 33 },
      { name: "qa", parts_in: 0, parts_out: 0 },
    ],
  },
};

export const SingleStep: Story = {
  name: "Single step",
  args: {
    steps: [{ tool: "pseudo-translate" }],
    runState: "running",
    snapshots: [{ name: "pseudo-translate", parts_in: 42, parts_out: 17 }],
  },
};

export const LongPipeline: Story = {
  name: "Long pipeline (6 steps)",
  args: {
    steps: [
      { tool: "recycle" },
      { tool: "term-lookup" },
      { tool: "translate" },
      { tool: "qa" },
      { tool: "term-enforce" },
      { tool: "tm-update" },
    ],
    runState: "running",
    snapshots: [
      { name: "recycle", parts_in: 200, parts_out: 200 },
      { name: "term-lookup", parts_in: 200, parts_out: 200 },
      { name: "translate", parts_in: 200, parts_out: 143 },
      { name: "qa", parts_in: 143, parts_out: 98 },
      { name: "term-enforce", parts_in: 0, parts_out: 0 },
      { name: "tm-update", parts_in: 0, parts_out: 0 },
    ],
  },
};

// ---------------------------------------------------------------------------
// Animated simulation
// ---------------------------------------------------------------------------

import { useState, useEffect, useCallback } from "react";

function AnimatedPipeline({
  steps,
  fileCount = 3,
  partsPerFile = 40,
  intervalMs = 50,
}: {
  steps: { tool: string }[];
  fileCount?: number;
  partsPerFile?: number;
  intervalMs?: number;
}) {
  const totalParts = fileCount * partsPerFile;
  const [snapshots, setSnapshots] = useState<StepSnapshot[]>(
    steps.map((s) => ({ name: s.tool, parts_in: 0, parts_out: 0 })),
  );
  const [runState, setRunState] = useState<"idle" | "running" | "complete">("idle");

  const run = useCallback(() => {
    setRunState("running");
    setSnapshots(steps.map((s) => ({ name: s.tool, parts_in: 0, parts_out: 0 })));

    let tick = 0;
    const timer = setInterval(() => {
      tick++;
      setSnapshots(
        steps.map((s, i) => {
          // Each step lags behind the previous by a few ticks.
          const lag = i * 8;
          const inProgress = Math.min(totalParts, Math.max(0, tick * 3 - lag));
          const outProgress = Math.min(inProgress, Math.max(0, tick * 3 - lag - 5));
          return { name: s.tool, parts_in: inProgress, parts_out: outProgress };
        }),
      );

      if (tick * 3 > totalParts + steps.length * 8 + 10) {
        clearInterval(timer);
        setSnapshots(
          steps.map((s) => ({ name: s.tool, parts_in: totalParts, parts_out: totalParts })),
        );
        setRunState("complete");
      }
    }, intervalMs);

    return () => clearInterval(timer);
  }, [steps, totalParts, intervalMs]);

  useEffect(() => {
    const cleanup = run();
    return cleanup;
  }, [run]);

  return (
    <div className="space-y-4 p-4">
      <PipelineProgress steps={steps} snapshots={snapshots} runState={runState} />
      <p className="text-xs text-muted-foreground">
        {runState === "running" ? "Processing..." : runState === "complete" ? "Done!" : "Idle"}
      </p>
    </div>
  );
}

export const Animated: Story = {
  name: "Animated simulation",
  render: () => <AnimatedPipeline steps={fourSteps} fileCount={3} partsPerFile={40} />,
};
