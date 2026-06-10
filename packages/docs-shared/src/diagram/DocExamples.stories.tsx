import type { Meta, StoryObj } from "@storybook/react-vite";
import { PipelineDiagram } from "./PipelineDiagram";
import { StreamDiagram } from "./StreamDiagram";
import { PhaseFlow } from "./PhaseFlow";
import { RedactionDiagram } from "./RedactionDiagram";
import { withDiagramTheme } from "./storyEnv";

/*
  The diagrams exactly as they appear on the docs pages — kept here so the page
  visuals have a home in Storybook and a place to review light/dark before
  publishing. Each story mirrors a converted ASCII diagram (formerly a code
  fence) one-for-one.
*/
const meta: Meta = {
  title: "Diagrams/Doc examples",
  decorators: [withDiagramTheme],
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

// framework/redaction.md
export const Redaction: Story = {
  name: "Redaction — framework/redaction",
  render: () => (
    <RedactionDiagram
      original="Mr Bean is the new King of England"
      redact={["Mr Bean", "King of England"]}
      translated="M. Bean est le nouveau Roi d'Angleterre"
    />
  ),
};

// framework/content-preparation.md
export const ContentPreparation: Story = {
  name: "Content-preparation pass — framework/content-preparation",
  render: () => (
    <PipelineDiagram
      channelLabel=""
      stages={[
        { label: "source", role: "io" },
        { label: "settle", sub: "transformers" },
        { label: "segment", sub: "sentence overlay", role: "annotate" },
        { label: "recognize", sub: "terms · entities", role: "annotate" },
        { label: "check", sub: "QA findings", role: "qa" },
        { label: "translate", sub: "TM · AI · MT", role: "translate" },
      ]}
    />
  ),
};

// framework/segmentation.md
export const Segmentation: Story = {
  name: "Segmentation overlay — framework/segmentation",
  render: () => (
    <StreamDiagram
      title="segment (overlay only — source runs unchanged)"
      items={[
        { kind: "Block source", detail: '"Dr. Smith arrived. He was late."', role: "block" },
        { kind: "segmentation overlay", role: "meta", note: "anchored to run-index ranges" },
        { kind: "segment", detail: '[0 … 18] · "Dr. Smith arrived."', depth: 1, role: "layer" },
        { kind: "segment", detail: '[19 … 31] · "He was late."', depth: 1, role: "layer" },
      ]}
    />
  ),
};

// react/pipeline.md — "Round-trip in one diagram"
export const ReactRoundTrip: Story = {
  name: "kapi-react round-trip — react/pipeline",
  render: () => (
    <PhaseFlow
      nodes={[
        { label: "src/App.tsx", sub: "<h1>Welcome</h1>" },
        {
          label: "i18n/ Block",
          sub: 'hash "aB3" · source + targets',
          edge: "kapi-react extract (source only)",
          role: "io",
          loop: ["kapi ai-translate --target-lang fr", "then de … (additive, in place)"],
        },
        {
          label: "public/translations/{locale}.json",
          sub: '{ "aB3": "Bienvenue" }',
          edge: "compile",
          role: "translate",
        },
        {
          label: "dist/translations/{locale}/",
          sub: "index.json + lazy chunks",
          edge: "kapi-react split (optional)",
        },
        {
          label: 'Your app renders "Bienvenue"',
          edge: "loadTranslations / loadTranslationChunk",
          role: "io",
        },
      ]}
    />
  ),
};

// contribute/architecture/001-vision-and-modules.md
export const ModuleDependencies: Story = {
  name: "Module dependency layering — AD-001",
  render: () => (
    <PipelineDiagram
      channelLabel=""
      caption="Each module depends only on those to its left; CI enforces the boundaries."
      stages={[
        { label: "framework", sub: "core/ · no platform deps", role: "io" },
        { label: "cli", sub: "shared CLI base" },
        {
          lanes: [{ label: "kapi" }, { label: "kapi-desktop" }],
          parallelLabel: "depend on framework + cli",
        },
      ]}
    />
  ),
};
