// Curated lab scenarios for the flow workspace: each is a complete, runnable
// teaching setup — a flow, an input sample, and (for the project-scope story)
// project-level tool presets (the recipe's defaults.tools). Selecting one
// loads it into the editor; running it replays the trace on the same nodes,
// and the run inspector shows what each step attached (AD-002).

import type { FlowSpec } from "@neokapi/flow-editor";

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
  },
  {
    id: "segmentation",
    label: "Segmentation",
    description:
      "Sentence segmentation is a stand-off overlay, not a structural split: the segmentation step attaches sentence spans over the source (open the run inspector to see them), and later steps work per segment without the text ever being cut apart.",
    steps: [{ tool: "segmentation" }, { tool: "ai-translate" }, { tool: "qa-check" }],
    sampleId: "support-reply",
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
  },
  {
    id: "pseudo",
    label: "Pseudo-translation",
    description:
      "The quickest end-to-end pipeline: pseudo-translate writes an accented, padded target for every block so layout and encoding issues surface before any real translation is bought.",
    steps: [{ tool: "pseudo-translate" }, { tool: "word-count" }],
    sampleId: "support-reply",
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
