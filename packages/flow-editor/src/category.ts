// Category visual identity — color-coded rails for each tool type.
//
// Keyed off the canonical `ToolCategory` vocabulary generated from Go
// (@neokapi/contract-types). The `Record<ToolCategory, …>` makes a missing
// style a compile error, so the editor can never silently fall back to gray
// for a whole category again. Hues are intentionally distinct per category but
// warm-shifted to complement the Sandstone theme's earthy palette.

import type { ToolCategory } from "./types";
import {
  Languages,
  ShieldCheck,
  BarChart3,
  Type,
  RefreshCw,
  Workflow,
  type LucideIcon,
} from "lucide-react";

export interface CategoryStyle {
  /** OKLCH border/accent color */
  color: string;
  /** Lighter background variant */
  bg: string;
  /** Text-on-dark variant */
  text: string;
  /** Human-readable label */
  label: string;
  /** Lucide icon component */
  icon: LucideIcon;
}

const CATEGORIES: Record<ToolCategory, CategoryStyle> = {
  translation: {
    color: "oklch(0.7 0.13 85)",
    bg: "oklch(0.7 0.13 85 / 0.12)",
    text: "oklch(0.8 0.1 85)",
    label: "Translation",
    icon: Languages,
  },
  quality: {
    color: "oklch(0.7 0.17 145)",
    bg: "oklch(0.7 0.17 145 / 0.12)",
    text: "oklch(0.8 0.13 145)",
    label: "Quality",
    icon: ShieldCheck,
  },
  analysis: {
    color: "oklch(0.75 0.1 55)",
    bg: "oklch(0.75 0.1 55 / 0.12)",
    text: "oklch(0.82 0.08 55)",
    label: "Analysis",
    icon: BarChart3,
  },
  "text-processing": {
    color: "oklch(0.65 0.14 300)",
    bg: "oklch(0.65 0.14 300 / 0.12)",
    text: "oklch(0.78 0.1 300)",
    label: "Text Processing",
    icon: Type,
  },
  convert: {
    color: "oklch(0.7 0.12 180)",
    bg: "oklch(0.7 0.12 180 / 0.12)",
    text: "oklch(0.8 0.08 180)",
    label: "Convert",
    icon: RefreshCw,
  },
  pipeline: {
    color: "oklch(0.6 0.02 106)",
    bg: "oklch(0.6 0.02 106 / 0.12)",
    text: "oklch(0.7 0.01 106)",
    label: "Pipeline",
    icon: Workflow,
  },
};

// Legacy / non-normalized category strings that may still arrive from
// un-regenerated plugin manifests. Backend NormalizeCategory handles the
// generated dataset; this is the frontend resilience guard.
const ALIASES: Record<string, ToolCategory> = {
  translate: "translation",
  validate: "quality",
  transform: "text-processing",
  enrich: "analysis",
};

export function getCategoryStyle(category: string): CategoryStyle {
  if (category in CATEGORIES) return CATEGORIES[category as ToolCategory];
  const alias = ALIASES[category];
  return alias ? CATEGORIES[alias] : CATEGORIES.pipeline;
}

export function getCategoryColor(category: string): string {
  return getCategoryStyle(category).color;
}

export const ALL_CATEGORIES = (Object.entries(CATEGORIES) as [ToolCategory, CategoryStyle][]).map(
  ([id, style]) => ({ id, ...style }),
);
