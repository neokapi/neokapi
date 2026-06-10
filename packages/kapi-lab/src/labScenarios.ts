// Curated lab scenarios for the flow workspace: each is a complete, runnable
// teaching setup — a flow, an input sample, and (for the project-scope story)
// project-level tool presets (the recipe's defaults.tools). Selecting one
// loads it into the editor; running it replays the trace on the same nodes,
// and the run inspector shows what each step attached (AD-002).

import type { FlowSpec } from "@neokapi/flow-editor";

/**
 * One step of a scenario's guided walkthrough. Advancing to a step applies its
 * editor focus (selecting a node/endpoint opens the matching panel and draws a
 * highlight ring) — the lesson literally points at the workspace instead of
 * describing it from the outside.
 */
export interface LessonStep {
  /** What to look at and why — shown in the walkthrough card. */
  prose: string;
  /**
   * Editor focus applied when the step activates: a node id (`tool-<i>`,
   * `endpoint-source`, `endpoint-sink`) or null to clear the selection.
   * Omitted = leave the current selection alone.
   */
  select?: string | null;
  /** Panel for a tool-node focus (default "inspect"). */
  mode?: "inspect" | "configure";
  /** This step's primary action is running the flow (the card offers Run). */
  run?: boolean;
}

export interface LabScenario {
  id: string;
  label: string;
  /** One-line teaching goal shown under the picker. */
  description: string;
  /** The flow loaded into the editor. */
  steps: FlowSpec["steps"];
  /**
   * Project-level tool presets (defaults.tools in the generated recipe). The
   * engine merges them under each step's own config — the step wins per key.
   */
  presets?: Record<string, Record<string, unknown>>;
  /** Sample file to preselect (id from SAMPLES). */
  sampleId?: string;
  /**
   * Guided walkthrough: ordered steps that drive the workspace (focus a node,
   * open a panel, run the flow). Scenarios without one are free play.
   */
  walkthrough?: LessonStep[];
}

export const LAB_SCENARIOS: LabScenario[] = [
  {
    id: "redaction",
    label: "Redaction",
    description:
      "Protect sensitive content before it reaches a model: redact replaces matched spans with placeholders and vaults the originals (watch the redaction.secret annotation appear), ai-translate works on the protected text, and unredact restores the originals into the translation.",
    steps: [
      { tool: "redact" },
      { tool: "ai-translate" },
      { tool: "qa-check" },
      { tool: "unredact" },
    ],
    // The redaction rules live at PROJECT level: every flow in this project
    // redacts the same names. The step stays bare — select it to see the
    // inherited preset in the config panel.
    presets: {
      redact: {
        detectors: ["rules"],
        rules: [
          { term: "Acme Corp", category: "org" },
          { term: "Jane Doe", category: "person" },
        ],
      },
    },
    sampleId: "support-reply",
    walkthrough: [
      {
        prose:
          "The redact step is bare, yet it knows what to redact: the rules live at PROJECT level (defaults.tools in the recipe), and every flow in the project inherits them. The config panel shows the inherited preset.",
        select: "tool-0",
        mode: "configure",
      },
      {
        prose:
          "redact is a recoverable transformer — it rewrites the source (placeholders in, originals vaulted) and the placement check keeps it ahead of any step that sends content off-machine. Run the flow.",
        run: true,
        select: null,
      },
      {
        prose:
          "What redact did: each block's after-text carries placeholders, and a redaction.secret annotation records what was vaulted. The model downstream only ever sees the placeholders.",
        select: "tool-0",
        mode: "inspect",
      },
      {
        prose:
          "unredact restores the vaulted originals into the translated target — the names are back, in the right places, in the translation.",
        select: "tool-3",
        mode: "inspect",
      },
      {
        prose:
          "And the written file: the Sink shows the output with its Native lines diffed against the input — structure intact, block text translated, secrets restored.",
        select: "endpoint-sink",
      },
    ],
  },
  {
    id: "redaction-ner",
    label: "Redaction (on-device NER)",
    description:
      "Entity-driven redaction with nothing leaving your browser: a GLiNER model (ONNX, run by onnxruntime-web) detects people, organizations and locations on-device, redact replaces them using the entity overlay, and unredact restores the originals after translation. Because the detector is local, the placement check has no remote egress to object to — the first run downloads the model (~175 MB) once.",
    steps: [
      { tool: "ai-entity-extract", config: { engine: "ner" } },
      {
        tool: "redact",
        config: { detectors: ["entities"], entityTypes: ["person", "org", "location"] },
      },
      { tool: "ai-translate" },
      { tool: "unredact" },
    ],
    sampleId: "support-reply",
    walkthrough: [
      {
        prose:
          "Same protection, but entity-driven and fully on-device: ai-entity-extract runs a GLiNER model in your browser (engine: ner) — nothing leaves the page, so the placement check has no remote egress to object to.",
        select: "tool-0",
        mode: "configure",
      },
      {
        prose:
          "Run it. The first run downloads the model (~175 MB, cached by the browser) — watch the progress bar under the canvas.",
        run: true,
        select: null,
      },
      {
        prose:
          "The extractor attached an entity overlay — the people, organizations and locations it found, with confidence scores.",
        select: "tool-0",
        mode: "inspect",
      },
      {
        prose:
          "redact consumed those entity spans and replaced them with placeholders; after translation, unredact restores the originals.",
        select: "tool-1",
        mode: "inspect",
      },
    ],
  },
  {
    id: "segmentation",
    label: "Segmentation",
    description:
      "Sentence segmentation is a stand-off overlay, not a structural split: the segmentation step attaches sentence spans over the source (open the run inspector to see them), and later steps work per segment without the text ever being cut apart.",
    steps: [{ tool: "segmentation" }, { tool: "ai-translate" }, { tool: "qa-check" }],
    sampleId: "support-reply",
    walkthrough: [
      {
        prose:
          "Start at the Source: the reader hands the first tool whole blocks — one run of text each, no sentence boundaries yet.",
        select: "endpoint-source",
      },
      { prose: "Run the flow.", run: true, select: null },
      {
        prose:
          "Open the segmentation step: each block now carries sentence spans — a stand-off overlay anchored to the runs. The text itself was never cut apart.",
        select: "tool-0",
        mode: "inspect",
      },
      {
        prose:
          "Later steps work per segment: ai-translate wrote the target sentence by sentence, guided by those spans.",
        select: "tool-1",
        mode: "inspect",
      },
    ],
  },
  {
    id: "annotations",
    label: "Content model & annotations",
    description:
      "Tools communicate through stand-off state on the block: segmentation adds sentence spans, term-check and qa-check attach qa findings, ai-translate writes the target. Step through the run and click each node to watch the block accumulate overlays and annotations.",
    steps: [
      { tool: "segmentation" },
      { tool: "ai-translate" },
      { tool: "term-check" },
      { tool: "qa-check" },
    ],
    sampleId: "support-reply",
    walkthrough: [
      {
        prose:
          "Start at the Source: the reader turns the file into the content model — Layers and Groups containing Blocks whose text is a sequence of Runs. Inspect the tree; it is what the first tool receives.",
        select: "endpoint-source",
      },
      {
        prose:
          "Run the flow, then watch one block accumulate stand-off state as it passes each step — nothing rewrites the text; tools communicate by attaching overlays and annotations.",
        run: true,
        select: null,
      },
      {
        prose: "segmentation attached sentence spans over the source runs.",
        select: "tool-0",
        mode: "inspect",
      },
      {
        prose:
          "ai-translate wrote the fr target — a first-class, locale-keyed record on the block.",
        select: "tool-1",
        mode: "inspect",
      },
      {
        prose:
          "term-check and qa-check attach findings without touching text or target — open each and read the +overlay / +annotation delta chips.",
        select: "tool-2",
        mode: "inspect",
      },
      {
        prose:
          "Now scrub the transport under the canvas to replay the run — the dots on the edges are the parts in flight between steps.",
        select: null,
      },
    ],
  },
  {
    id: "pseudo",
    label: "Pseudo-translation",
    description:
      "The quickest end-to-end pipeline: pseudo-translate writes an accented, padded target for every block so layout and encoding issues surface before any real translation is bought.",
    steps: [{ tool: "pseudo-translate" }, { tool: "word-count" }],
    sampleId: "support-reply",
    walkthrough: [
      { prose: "The quickest end-to-end pipeline — run it.", run: true, select: null },
      {
        prose:
          "pseudo-translate wrote an accented, padded target for every block, so layout and encoding issues surface before any real translation is bought.",
        select: "tool-0",
        mode: "inspect",
      },
      {
        prose:
          "Inspect the Sink: the Native tab diffs the written file against the input — the structure is byte-identical, only the block text changed. That is the round-trip guarantee.",
        select: "endpoint-sink",
      },
    ],
  },
  {
    id: "scripting",
    label: "Scripting",
    description:
      "When no built-in tool fits, the script step runs a small JavaScript program over each Part — modify text, filter parts, or log() what flows through. The step IS the script: its code editor (with typed completions) lives in the step's config panel.",
    steps: [
      {
        tool: "script",
        config: {
          // Source is immutable by default (AD-006); a script that rewrites it
          // must opt in, making the step an explicit transformer.
          allowSourceMutation: true,
          code: '// process(part) runs once for every Part in the document.\n/** @param {Part} part */\nfunction process(part) {\n  if (part.type === "block") {\n    part.block.source[0].content.text =\n      part.block.source[0].content.text.toUpperCase();\n  }\n  return part;\n}\n',
        },
      },
      { tool: "word-count" },
    ],
    sampleId: "support-reply",
    walkthrough: [
      {
        prose:
          "A script step is a tool you write yourself: open its config and the panel is a real code editor — typed completions for part / emit / skip / log, plus a library of examples to start from. This one rewrites the source, so it declares allowSourceMutation — the source is read-only for scripts unless they opt into being a transformer.",
        select: "tool-0",
        mode: "configure",
      },
      {
        prose: "Run the flow — the engine executes your exact code over every part.",
        run: true,
        select: null,
      },
      {
        prose:
          "What the script did: every block's text was rewritten (this one shouts). Edit the code — try a different example — and run again.",
        select: "tool-0",
        mode: "inspect",
      },
    ],
  },
  {
    id: "build-your-own",
    label: "Build your own",
    description:
      "Start from the classic prepare-translate-check chain and reshape it: add tools from the palette, reorder, configure — the placement check flags an unsafe transformer slot before you run.",
    steps: [
      {
        tool: "search-replace",
        config: { pairs: [{ search: "color", replace: "colour" }], source: true, target: false },
      },
      {
        tool: "redact",
        config: {
          detectors: ["rules"],
          rules: [
            { term: "Acme Corp", category: "org" },
            { term: "Jane Doe", category: "person" },
          ],
        },
      },
      { tool: "ai-translate" },
      { tool: "qa-check" },
    ],
    sampleId: "support-reply",
  },
];
