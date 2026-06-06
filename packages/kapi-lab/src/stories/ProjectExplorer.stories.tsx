import type { Meta, StoryObj } from "@storybook/react-vite";
import ProjectExplorer, { FLOWS, TARGETS, recipeFor } from "../ProjectExplorer";
import { WORKSPACE_SAMPLES, workspaceSampleById } from "../workspaceSamples";

// ProjectExplorer drives the live .kapi project lifecycle (extract → run a
// declared flow → merge) against the real kapi WASM engine, which is supplied
// by the docs host as `assets` (the wasm-exec + wasm URLs). Storybook does not
// serve those assets, so the live story renders the booting/idle shell; the
// "Recipe" stories below render the pure recipe + flow surface so the
// config-as-code output is reviewable without WASM.
const meta: Meta<typeof ProjectExplorer> = {
  title: "Lab/Project Explorer",
  component: ProjectExplorer,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectExplorer>;

// The live explorer with no WASM assets — renders its idle shell. In the docs
// the host passes real asset URLs and the three lifecycle steps light up.
export const Idle: Story = {
  args: { assets: null, defaultSampleId: "json" },
};

// Review the generated recipe (config-as-code) for each sample without WASM:
// source/target languages, the content glob, and every declared flow.
function RecipePreview({ sampleId }: { sampleId: string }) {
  const sample = workspaceSampleById(sampleId);
  return (
    <div style={{ maxWidth: 640 }}>
      <p style={{ fontSize: "0.85rem", opacity: 0.8 }}>
        {sample.label} ({sample.kind}) — targets {TARGETS.join(", ")}, flows{" "}
        {FLOWS.map((f) => f.id).join(", ")}
      </p>
      <pre
        style={{
          background: "var(--ifm-background-surface-color, #f6f7f9)",
          padding: "0.8rem",
          borderRadius: 8,
          fontSize: "0.8rem",
          overflow: "auto",
        }}
      >
        {recipeFor(sample)}
      </pre>
    </div>
  );
}

export const RecipeJson: Story = {
  name: "Recipe — JSON catalog",
  render: () => <RecipePreview sampleId="json" />,
};

export const RecipeDocx: Story = {
  name: "Recipe — Word document",
  render: () => <RecipePreview sampleId="docx" />,
};

export const RecipeAll: Story = {
  name: "Recipe — every sample",
  render: () => (
    <div style={{ display: "grid", gap: "1rem" }}>
      {WORKSPACE_SAMPLES.map((w) => (
        <RecipePreview key={w.id} sampleId={w.id} />
      ))}
    </div>
  ),
};
