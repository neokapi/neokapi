import type { Meta, StoryObj } from "@storybook/react-vite";
import { ArchitectureDiagram } from "./ArchitectureDiagram";
import { PipelineDiagram } from "./PipelineDiagram";
import { StreamDiagram } from "./StreamDiagram";
import { RoundTripDiagram } from "./RoundTripDiagram";
import { PhaseFlow } from "./PhaseFlow";
import { LanesDiagram } from "./LanesDiagram";
import { SwimlaneDiagram } from "./SwimlaneDiagram";
import { RedactionDiagram } from "./RedactionDiagram";
import { withDiagramTheme } from "./storyEnv";

/*
  The diagram kit (`@neokapi/docs-shared`) — the uniform, theme-aware SVG visual
  language used across the docs sites. One SVG renders in both light and dark;
  toggle the Storybook theme to check both. Every doc diagram is built from these
  primitives, so this section is the catalogue and the regression surface for the
  look. See "Diagrams/Doc examples" for the exact diagrams used on doc pages.
*/
const meta: Meta = {
  title: "Diagrams/Components",
  decorators: [withDiagramTheme],
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

export const Architecture: Story = {
  name: "ArchitectureDiagram (hero)",
  render: () => <ArchitectureDiagram animated />,
};

export const Pipeline: Story = {
  name: "PipelineDiagram (linear)",
  render: () => (
    <PipelineDiagram
      stages={[
        { label: "RawDocument" },
        { label: "Reader", sub: "DataFormat", role: "io" },
        { label: "segmentation", role: "annotate" },
        { label: "ai-translate", role: "translate" },
        { label: "qa-check", role: "qa" },
        { label: "Writer", sub: "DataFormat", role: "io" },
      ]}
    />
  ),
};

export const PipelineFanOut: Story = {
  name: "PipelineDiagram (fan-out lanes)",
  render: () => (
    <PipelineDiagram
      caption="A stage can fan out into parallel lanes (workers / branches)."
      stages={[
        { label: "Reader", sub: "DataFormat", role: "io" },
        {
          lanes: [
            { label: "ai-translate", sub: "fr" },
            { label: "ai-translate", sub: "de" },
            { label: "ai-translate", sub: "ja" },
          ],
          parallelLabel: "fan-out · N goroutines",
        },
        { label: "Writer", sub: "DataFormat", role: "io" },
      ]}
    />
  ),
};

export const Stream: Story = {
  name: "StreamDiagram (Part sequence)",
  render: () => (
    <StreamDiagram
      title="Read(ctx)"
      items={[
        { kind: "PartLayerStart", detail: 'format = "json"', role: "layer" },
        { kind: "PartBlock", detail: '"title"', depth: 1, role: "block" },
        {
          kind: "PartLayerStart",
          detail: 'embedded "html"',
          depth: 1,
          role: "layer",
          note: "child layer",
        },
        { kind: "PartBlock", detail: '"<b>Hello</b>"', depth: 2, role: "block" },
        { kind: "PartLayerEnd", depth: 1, role: "end" },
        { kind: "PartLayerEnd", role: "end" },
      ]}
    />
  ),
};

export const RoundTrip: Story = {
  name: "RoundTripDiagram (extract / merge)",
  render: () => (
    <RoundTripDiagram
      forward={[
        { label: "authored source", role: "io" },
        { label: "extract" },
        { label: "XLIFF / PO" },
        { label: "translator" },
      ]}
      back={[
        { label: "kapi merge", role: "io" },
        { label: "translated XLIFF / PO" },
        { label: "returned" },
      ]}
      hub={{ label: "project TM", sub: "pre-fill · absorb" }}
      forwardLabel="pre-fill"
      backLabel="absorb"
    />
  ),
};

export const Phases: Story = {
  name: "PhaseFlow (vertical, looping node)",
  render: () => (
    <PhaseFlow
      nodes={[
        { label: "Your source code" },
        {
          label: "i18n/",
          sub: "KLF archive",
          edge: "kapi-react extract",
          role: "io",
          loop: ["kapi ai-translate / pseudo / qa", "accumulate locales in place"],
        },
        {
          label: "public/translations/{locale}.json",
          edge: "kapi-react compile",
          role: "translate",
        },
      ]}
    />
  ),
};

export const Lanes: Story = {
  name: "LanesDiagram (thread handoff)",
  render: () => (
    <LanesDiagram
      handoff="eventQueue"
      lanes={[
        {
          title: "Reader Thread",
          sub: "filterPool, bounded",
          role: "io",
          steps: ["read source bytes", "run the Okapi filter", "emit events"],
        },
        {
          title: "Writer Thread",
          sub: "writerPool, unbounded",
          role: "translate",
          steps: ["receive events", "re-assemble the skeleton", "write target bytes"],
        },
      ]}
    />
  ),
};

export const Swimlane: Story = {
  name: "SwimlaneDiagram (message passing)",
  render: () => (
    <SwimlaneDiagram
      actors={[
        { label: "Developer", sub: "kapi CLI", role: "io" },
        { label: "Bowrain Server", role: "translate" },
        { label: "Translator", role: "qa" },
      ]}
      messages={[
        { from: 0, to: 1, label: "kapi push" },
        { from: 1, to: 1, label: "translate / QA" },
        { from: 1, to: 2, label: "assign" },
        { from: 2, to: 1, label: "review, approve" },
        { from: 0, to: 1, label: "kapi pull" },
      ]}
    />
  ),
};

export const Redaction: Story = {
  name: "RedactionDiagram (black-out bars)",
  render: () => (
    <RedactionDiagram
      original="Mr Bean is the new King of England"
      redact={["Mr Bean", "King of England"]}
      translated="M. Bean est le nouveau Roi d'Angleterre"
      caption="Sensitive spans are blacked out before translation, then restored locally."
    />
  ),
};
