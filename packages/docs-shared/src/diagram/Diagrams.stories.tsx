import type { Meta, StoryObj } from "@storybook/react-vite";
import { ArchitectureDiagram } from "./ArchitectureDiagram";
import { PipelineDiagram } from "./PipelineDiagram";
import { StreamDiagram } from "./StreamDiagram";
import { RoundTripDiagram } from "./RoundTripDiagram";
import { PhaseFlow } from "./PhaseFlow";
import { LanesDiagram } from "./LanesDiagram";
import { SwimlaneDiagram } from "./SwimlaneDiagram";
import { RedactionDiagram } from "./RedactionDiagram";
import { AxisLadderDiagram } from "./AxisLadderDiagram";
import { AxisFamiliesDiagram } from "./AxisFamiliesDiagram";
import { CycleDiagram } from "./CycleDiagram";
import { TypeDiagram } from "./TypeDiagram";
import { GatedLoopDiagram } from "./GatedLoopDiagram";
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
        { label: "translate", role: "translate" },
        { label: "qa", role: "qa" },
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
            { label: "translate", sub: "fr" },
            { label: "translate", sub: "de" },
            { label: "translate", sub: "ja" },
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
        {
          kind: "PartBlock",
          detail: '"<b>Hello</b>"',
          depth: 2,
          role: "block",
        },
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
      hub={{ label: "project content memory", sub: "pre-fill · absorb" }}
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
          sub: "KBF archive",
          edge: "neokapi-i18n extract",
          role: "io",
          loop: ["kapi translate / pseudo / qa", "accumulate locales in place"],
        },
        {
          label: "public/translations/{locale}.json",
          edge: "neokapi-i18n compile",
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
        { from: 1, to: 1, label: "translate / checks" },
        { from: 1, to: 2, label: "assign" },
        { from: 2, to: 1, label: "review, approve" },
        { from: 0, to: 1, label: "kapi pull" },
      ]}
    />
  ),
};

export const Redaction: Story = {
  name: "RedactionDiagram (marker black-out + category chips)",
  render: () => (
    <RedactionDiagram
      original="Email Sarah Chen the Project Halcyon launch date."
      redact={[{ text: "Sarah Chen", label: "Person" }, "Project Halcyon"]}
      translated="Envoyez à Sarah Chen la date de lancement de Project Halcyon."
      caption="A span is either blacked out with a marker (sensitive) or shown as a labeled category chip."
    />
  ),
};

export const AxisLadder: Story = {
  name: "AxisLadderDiagram (ascending rungs)",
  render: () => (
    <AxisLadderDiagram
      rungs={[
        { grade: "G0", name: "opaque", gloss: "bytes only" },
        { grade: "G1", name: "metadata", gloss: "title, author, page count" },
        { grade: "G2", name: "linear text", gloss: "reading-order characters" },
        {
          grade: "G3",
          name: "roles",
          gloss: "headings, tables, reading order",
        },
        { grade: "G4", name: "geometry", gloss: "page coords, bounding boxes" },
      ]}
      caption="Structure & Geometry — how much document structure we recover, rung by rung."
    />
  ),
};

export const ConvergenceTargetLadder: Story = {
  name: "AxisLadderDiagram (convergence — target lifecycle)",
  render: () => (
    <AxisLadderDiagram
      rungs={[
        {
          grade: "1",
          name: "draft",
          gloss: "machine placeholder or fuzzy leverage",
        },
        { grade: "2", name: "translated", gloss: "a real translation exists" },
        {
          grade: "3",
          name: "reviewed",
          gloss: "a person approved this exact pair",
        },
        { grade: "4", name: "signed-off", gloss: "final" },
      ]}
      caption="Target lifecycle — how far a translation has progressed."
    />
  ),
};

export const ConvergenceSourceLadder: Story = {
  name: "AxisLadderDiagram (convergence — source readiness)",
  render: () => (
    <AxisLadderDiagram
      rungs={[
        { grade: "1", name: "authored", gloss: "source content exists" },
        {
          grade: "2",
          name: "checked",
          gloss: "clears its brand & terminology checks",
        },
        {
          grade: "3",
          name: "approved",
          gloss: "a person signed off on the source",
        },
      ]}
      caption="Source authoring readiness — the source-side counterpart."
    />
  ),
};

export const AxisFamilies: Story = {
  name: "AxisFamiliesDiagram (axes grouped by family)",
  render: () => (
    <AxisFamiliesDiagram
      families={[
        {
          name: "Comprehension",
          tagline: "how deeply we read it",
          axes: [
            { label: "Engine", range: "L0–L4" },
            { label: "Vocabulary", range: "V0–V3" },
            { label: "Structure & Geometry", range: "G0–G4" },
          ],
        },
        {
          name: "Assurance",
          tagline: "how we prove it",
          axes: [
            { label: "Corpus", range: "C0–C3" },
            { label: "Security", range: "S0–S4" },
          ],
        },
        {
          name: "Enablement",
          tagline: "how we work with it",
          axes: [
            { label: "Knowledge", range: "K0–K3" },
            { label: "Editor", range: "E0–E4" },
          ],
        },
      ]}
      caption="The maturity axes group by the question they answer."
    />
  ),
};

export const Cycle: Story = {
  name: "CycleDiagram (closed runbook loop)",
  render: () => (
    <CycleDiagram
      steps={[
        { label: "Reconcile", sub: "ledger vs reality" },
        { label: "Compute due", sub: "signals + watermarks" },
        { label: "Rank & budget" },
        { label: "Execute", sub: "with evidence" },
        { label: "Record", sub: "ledger commit" },
        { label: "Reflect", sub: "learnings" },
      ]}
      caption="The format-ops runbook is a self-feeding loop; each run records what it consumed."
    />
  ),
};

export const Types: Story = {
  name: "TypeDiagram (record types and their links)",
  render: () => (
    <TypeDiagram
      caption="A type card lists its fields; an edge carries what the link between two types means."
      boxes={[
        {
          name: "Layer",
          col: 0,
          role: "io",
          self: "nests — embedded content",
          fields: [
            { name: "Format", type: "string" },
            { name: "ParentID", type: "string" },
          ],
        },
        {
          name: "Block",
          col: 1,
          role: "translate",
          fields: [
            { name: "Translatable", type: "bool" },
            { name: "Source", type: "[]Run" },
            { name: "Targets", type: "map[VariantKey]*Target" },
            { name: "Overlays", type: "[]Overlay" },
          ],
        },
        {
          name: "Run",
          col: 2,
          fields: [
            { name: "Text", type: "*TextRun" },
            { name: "Ph", type: "*PlaceholderRun" },
          ],
        },
        {
          name: "Target",
          col: 2,
          role: "translate",
          fields: [
            { name: "Runs", type: "[]Run" },
            { name: "Status", type: "TargetStatus" },
          ],
        },
        {
          name: "Overlay",
          col: 2,
          role: "annotate",
          fields: [
            { name: "Type", type: "OverlayType" },
            { name: "Spans", type: "[]Span" },
          ],
        },
      ]}
      edges={[
        { from: 0, to: 1, label: "contains" },
        { from: 1, to: 2, label: "flat Source sequence" },
        { from: 1, to: 3, label: "per variant" },
        { from: 1, to: 4, label: "stand-off, run-anchored" },
      ]}
    />
  ),
};

export const GatedLoop: Story = {
  name: "GatedLoopDiagram (source-first ship-gates)",
  render: () => (
    <GatedLoopDiagram
      nodes={[
        {
          kind: "phase",
          role: "source",
          label: "settle source",
          steps: ["term-check + protect", "voice-check", "source checks"],
        },
        {
          kind: "gate",
          label: "source ship-gate",
          sub: "source_gate",
          hold: "hold — settle your source first",
        },
        {
          kind: "phase",
          role: "translate",
          label: "translate approved source",
          steps: ["recycle (content memory-first)", "AI remainder", "target checks"],
        },
        {
          kind: "gate",
          label: "target ship-gate",
          sub: "ship_gate",
          hold: "park — needs review",
        },
        { kind: "done", label: "converged" },
      ]}
      caption="Get the source right, gate it, then translate the approved source per locale — a gate that is not met holds the work for a person rather than shipping."
    />
  ),
};

export const GatedLoopMonolingual: Story = {
  name: "GatedLoopDiagram (source-only — monolingual)",
  render: () => (
    <GatedLoopDiagram
      nodes={[
        {
          kind: "phase",
          role: "source",
          label: "settle source",
          steps: ["term-check + protect", "voice-check", "source checks"],
        },
        {
          kind: "gate",
          label: "source ship-gate",
          sub: "source_gate",
          hold: "hold — settle your source first",
        },
        { kind: "done", label: "on brand" },
      ]}
      caption="A monolingual project stops after the first half: settle the source, clear its gate."
    />
  ),
};
