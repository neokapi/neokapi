// Built-in flow templates — common patterns for localization workflows.

import type { FlowSpec, ToolCategory } from "./types";

export interface FlowTemplate {
  id: string;
  name: string;
  description: string;
  category: ToolCategory;
  /** Number of sequential + parallel steps (for complexity indicator). */
  stepCount: number;
  hasParallel: boolean;
  spec: FlowSpec;
}

export const FLOW_TEMPLATES: FlowTemplate[] = [
  {
    id: "translate-qa",
    name: "Translate + QA",
    description: "AI-translate content then run quality checks to catch errors.",
    category: "translation",
    stepCount: 2,
    hasParallel: false,
    spec: {
      steps: [
        { tool: "ai-translate", label: "Translate" },
        { tool: "qa-check", label: "Quality Check" },
      ],
    },
  },
  {
    id: "tm-translate",
    name: "TM Leverage + Translate",
    description: "Leverage translation memory first, then AI-translate unmatched segments.",
    category: "translation",
    stepCount: 2,
    hasParallel: false,
    spec: {
      steps: [
        { tool: "tm-leverage", label: "TM Leverage" },
        { tool: "ai-translate", label: "AI Translate" },
      ],
    },
  },
  {
    id: "pseudo-validate",
    name: "Pseudo-translate + Validate",
    description: "Generate pseudo-translations for testing, then validate for issues.",
    category: "quality",
    stepCount: 2,
    hasParallel: false,
    spec: {
      steps: [
        { tool: "pseudo-translate", label: "Pseudo-translate" },
        { tool: "qa-check", label: "Validate" },
      ],
    },
  },
  {
    id: "parallel-qa",
    name: "Translate + Parallel QA",
    description: "Translate, then run QA and brand checks in parallel for faster validation.",
    category: "quality",
    stepCount: 4,
    hasParallel: true,
    spec: {
      steps: [
        { tool: "ai-translate", label: "Translate" },
        {
          tool: "",
          parallel: [
            { tool: "qa-check", label: "QA Check" },
            { tool: "brand-vocab-check", label: "Brand Check" },
          ],
        },
        { tool: "word-count", label: "Word Count" },
      ],
    },
  },
  {
    id: "full-pipeline",
    name: "Full Pipeline",
    description:
      "Complete workflow: TM leverage, AI translate, parallel QA + brand check, word count.",
    category: "pipeline",
    stepCount: 5,
    hasParallel: true,
    spec: {
      steps: [
        { tool: "tm-leverage", label: "TM Leverage" },
        { tool: "ai-translate", label: "AI Translate" },
        {
          tool: "",
          parallel: [
            { tool: "qa-check", label: "QA Check" },
            { tool: "brand-vocab-check", label: "Brand Check" },
          ],
        },
        { tool: "word-count", label: "Word Count" },
      ],
    },
  },
  {
    id: "enrich-only",
    name: "Entity Extraction",
    description: "Extract named entities and terminology from content for review.",
    category: "analysis",
    stepCount: 2,
    hasParallel: true,
    spec: {
      steps: [
        {
          tool: "",
          parallel: [
            { tool: "entity-extract", label: "Entities" },
            { tool: "term-lookup", label: "Terminology" },
          ],
        },
      ],
    },
  },
];
