// Story fixtures shared by the flow-editor stories: a tool list that declares
// transformers (so placement and unmet-IO diagnostics fire), and the recorded
// run the run-review stories replay.

import type { ToolInfo } from "../types";
import type { FlowTrace } from "../traceTypes";
import toolsData from "../../../../apps/kapi-desktop/frontend/src/stories/fixtures/tools-metadata.json";

/** The desktop's tool metadata, as the palette and the nodes see it. */
export const tools = toolsData as ToolInfo[];

// Tools that declare isSourceTransform: true (transformer — rewrites the
// source) — in production these come from the backend as is_source_transform /
// recoverable, mapped to camelCase in the API layer.
export const transformerAwareTools: ToolInfo[] = [
  {
    name: "redact",
    display_name: "Redact",
    description: "Replace sensitive spans with placeholders before translation",
    category: "text-processing",
    has_schema: true,
    cardinality: "monolingual",
    consumes: [{ type: "entity", side: "source", optional: true }],
    produces: [
      { type: "source", side: "source" },
      { type: "redaction.secret", side: "source" },
    ],
    tags: ["privacy", "pre-processing"],
    isSourceTransform: true,
    recoverable: true,
  },
  {
    name: "unredact",
    display_name: "Unredact",
    description: "Restore the original spans after processing",
    category: "text-processing",
    cardinality: "monolingual",
    consumes: [{ type: "redaction.secret", side: "source" }],
    // unredact rewrites both sides coherently, so it produces the target port
    // and is exempt from the transformer-after-target placement rule.
    produces: [
      { type: "source", side: "source" },
      { type: "target", side: "target" },
    ],
    tags: ["privacy"],
    isSourceTransform: true,
  },
  {
    name: "source-normalise",
    display_name: "Source Normalise",
    description: "Normalise quotes, punctuation, and whitespace in source text",
    category: "text-processing",
    has_schema: true,
    cardinality: "monolingual",
    produces: [{ type: "source", side: "source" }],
    tags: ["text-processing", "pre-processing"],
    isSourceTransform: true,
  },
  {
    name: "case-transform",
    display_name: "Case Transform",
    description: "Rewrite source casing (upper, lower, title)",
    category: "text-processing",
    cardinality: "monolingual",
    produces: [{ type: "source", side: "source" }],
    tags: ["text-processing"],
    isSourceTransform: true,
  },
  {
    name: "entity-extract",
    display_name: "AI Entity Extract",
    description: "Recognize named entities with a cloud NER model",
    category: "analysis",
    cardinality: "monolingual",
    produces: [{ type: "entity", side: "source" }],
    side_effects: ["remote-source-egress"],
    tags: ["ai-powered"],
  },
  // Ordinary tools from the shared fixture, with the remote-egress effect on
  // translate so the placement stories exercise the egress rule.
  ...(toolsData as ToolInfo[])
    .filter((t) =>
      ["translate", "qa", "word-count", "pseudo-translate", "recycle"].includes(t.name),
    )
    .map((t) => (t.name === "translate" ? { ...t, side_effects: ["remote-source-egress"] } : t)),
];

/**
 * A run of the secure-translate flow (redact, translate) over two blocks, as
 * the engine records it: reader/writer nodes bracket the tool nodes and every
 * part is snapshotted after each tool.
 */
export const runReviewTrace: FlowTrace = {
  name: "lab",
  nodes: [
    { id: "reader", type: "reader", name: "read" },
    { id: "tool-1", type: "tool", name: "redact" },
    { id: "tool-2", type: "tool", name: "translate" },
    { id: "writer", type: "writer", name: "write" },
  ],
  events: [
    { ts: 120, type: "enter", nodeId: "tool-1", partId: "b1" },
    { ts: 480, type: "exit", nodeId: "tool-1", partId: "b1" },
    { ts: 510, type: "enter", nodeId: "tool-2", partId: "b1" },
    { ts: 2200, type: "exit", nodeId: "tool-2", partId: "b1" },
    { ts: 2300, type: "enter", nodeId: "tool-1", partId: "b2" },
    { ts: 2350, type: "exit", nodeId: "tool-1", partId: "b2" },
    { ts: 2400, type: "enter", nodeId: "tool-2", partId: "b2" },
    { ts: 3100, type: "exit", nodeId: "tool-2", partId: "b2" },
  ],
  parts: {
    b1: {
      initial: {
        id: "b1",
        type: "Block",
        summary: "Contact Jane Doe at Acme Corp",
        sourceText: "Contact Jane Doe at Acme Corp",
        detail: {
          overlays: [
            {
              type: "entity",
              side: "source",
              spans: [
                { start: 8, end: 16, text: "Jane Doe", note: "entity:person" },
                { start: 20, end: 29, text: "Acme Corp", note: "entity:organization" },
              ],
            },
          ],
        },
      },
      afterNode: {
        "tool-1": {
          id: "b1",
          type: "Block",
          summary: "Contact Jane Doe at Acme Corp",
          sourceText: "Contact [REDACTED:Person] at [REDACTED:Org]",
          detail: {
            annotations: [{ key: "redaction.secret", summary: "2 vaulted originals" }],
          },
        },
        "tool-2": {
          id: "b1",
          type: "Block",
          summary: "Contact Jane Doe at Acme Corp",
          sourceText: "Contact [REDACTED:Person] at [REDACTED:Org]",
          targetText: "Contactez [REDACTED:Person] chez [REDACTED:Org]",
          detail: {
            annotations: [{ key: "redaction.secret", summary: "2 vaulted originals" }],
          },
        },
      },
    },
    b2: {
      initial: {
        id: "b2",
        type: "Block",
        summary: "Thanks for reaching out!",
        sourceText: "Thanks for reaching out!",
      },
      afterNode: {
        "tool-1": {
          id: "b2",
          type: "Block",
          summary: "Thanks for reaching out!",
          sourceText: "Thanks for reaching out!",
        },
        "tool-2": {
          id: "b2",
          type: "Block",
          summary: "Thanks for reaching out!",
          sourceText: "Thanks for reaching out!",
          targetText: "Merci de nous avoir contactés !",
        },
      },
    },
  },
  durationUs: 3200,
};
