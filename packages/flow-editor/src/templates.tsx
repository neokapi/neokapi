// Built-in flow templates — common patterns for localization workflows.
//
// User-visible strings (template names, descriptions, step labels) are
// declared as lazy `get` accessors wrapping `t()` from the kapi-react
// runtime. The accessor defers the dictionary lookup to property-access
// time (i.e. render), so translations loaded after module evaluation
// still apply. A plain `label: t("…")` at module scope would freeze the
// source text before `loadTranslations()` resolves.

import { t } from "@neokapi/kapi-react/runtime";
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
    get name() {
      return t("Translate + QA", "flow template name");
    },
    get description() {
      return t("AI-translate content then run quality checks to catch errors.");
    },
    category: "translation",
    stepCount: 2,
    hasParallel: false,
    spec: {
      steps: [
        {
          tool: "translate",
          get label() {
            return t("Translate", "flow step label");
          },
        },
        {
          tool: "qa",
          get label() {
            return t("Quality Check", "flow step label");
          },
        },
      ],
    },
  },
  {
    id: "tm-translate",
    get name() {
      return t("TM Leverage + Translate", "flow template name");
    },
    get description() {
      return t("Leverage translation memory first, then AI-translate unmatched segments.");
    },
    category: "translation",
    stepCount: 2,
    hasParallel: false,
    spec: {
      steps: [
        {
          tool: "tm-leverage",
          get label() {
            return t("TM Leverage", "flow step label");
          },
        },
        {
          tool: "translate",
          get label() {
            return t("AI Translate", "flow step label");
          },
        },
      ],
    },
  },
  {
    id: "pseudo-validate",
    get name() {
      return t("Pseudo-translate + Validate", "flow template name");
    },
    get description() {
      return t("Generate pseudo-translations for testing, then validate for issues.");
    },
    category: "quality",
    stepCount: 2,
    hasParallel: false,
    spec: {
      steps: [
        {
          tool: "pseudo-translate",
          get label() {
            return t("Pseudo-translate", "flow step label");
          },
        },
        {
          tool: "qa",
          get label() {
            return t("Validate", "flow step label");
          },
        },
      ],
    },
  },
  {
    id: "parallel-qa",
    get name() {
      return t("Translate + Parallel QA", "flow template name");
    },
    get description() {
      return t("Translate, then run QA and brand checks in parallel for faster validation.");
    },
    category: "quality",
    stepCount: 4,
    hasParallel: true,
    spec: {
      steps: [
        {
          tool: "translate",
          get label() {
            return t("Translate", "flow step label");
          },
        },
        {
          tool: "",
          parallel: [
            {
              tool: "qa",
              get label() {
                return t("QA Check", "flow step label");
              },
            },
            {
              tool: "brand-vocab-check",
              get label() {
                return t("Brand Check", "flow step label");
              },
            },
          ],
        },
        {
          tool: "word-count",
          get label() {
            return t("Word Count", "flow step label");
          },
        },
      ],
    },
  },
  {
    id: "full-pipeline",
    get name() {
      return t("Full Pipeline", "flow template name");
    },
    get description() {
      return t(
        "Complete workflow: TM leverage, AI translate, parallel QA + brand check, word count.",
      );
    },
    category: "pipeline",
    stepCount: 5,
    hasParallel: true,
    spec: {
      steps: [
        {
          tool: "tm-leverage",
          get label() {
            return t("TM Leverage", "flow step label");
          },
        },
        {
          tool: "translate",
          get label() {
            return t("AI Translate", "flow step label");
          },
        },
        {
          tool: "",
          parallel: [
            {
              tool: "qa",
              get label() {
                return t("QA Check", "flow step label");
              },
            },
            {
              tool: "brand-vocab-check",
              get label() {
                return t("Brand Check", "flow step label");
              },
            },
          ],
        },
        {
          tool: "word-count",
          get label() {
            return t("Word Count", "flow step label");
          },
        },
      ],
    },
  },
  {
    id: "enrich-only",
    get name() {
      return t("Entity Extraction", "flow template name");
    },
    get description() {
      return t("Extract named entities and terminology from content for review.");
    },
    category: "analysis",
    stepCount: 2,
    hasParallel: true,
    spec: {
      steps: [
        {
          tool: "",
          parallel: [
            {
              tool: "entity-extract",
              get label() {
                return t("Entities", "flow step label");
              },
            },
            {
              tool: "term-lookup",
              get label() {
                return t("Terminology", "flow step label");
              },
            },
          ],
        },
      ],
    },
  },
];
