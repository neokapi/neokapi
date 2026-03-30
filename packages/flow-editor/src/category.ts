// Category visual identity — color-coded rails for each tool type.
// Hues are intentionally distinct per category but warm-shifted to
// complement the Sandstone theme's earthy palette.

import type { ToolCategory } from "./types";
import {
  Languages,
  ShieldCheck,
  Wand2,
  RefreshCw,
  Sparkles,
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
  translate: {
    color: "oklch(0.7 0.13 85)",
    bg: "oklch(0.7 0.13 85 / 0.12)",
    text: "oklch(0.8 0.1 85)",
    label: "Translate",
    icon: Languages,
  },
  validate: {
    color: "oklch(0.7 0.17 145)",
    bg: "oklch(0.7 0.17 145 / 0.12)",
    text: "oklch(0.8 0.13 145)",
    label: "Validate",
    icon: ShieldCheck,
  },
  transform: {
    color: "oklch(0.65 0.14 300)",
    bg: "oklch(0.65 0.14 300 / 0.12)",
    text: "oklch(0.78 0.1 300)",
    label: "Transform",
    icon: Wand2,
  },
  convert: {
    color: "oklch(0.7 0.12 180)",
    bg: "oklch(0.7 0.12 180 / 0.12)",
    text: "oklch(0.8 0.08 180)",
    label: "Convert",
    icon: RefreshCw,
  },
  enrich: {
    color: "oklch(0.75 0.1 55)",
    bg: "oklch(0.75 0.1 55 / 0.12)",
    text: "oklch(0.82 0.08 55)",
    label: "Enrich",
    icon: Sparkles,
  },
  pipeline: {
    color: "oklch(0.6 0.02 106)",
    bg: "oklch(0.6 0.02 106 / 0.12)",
    text: "oklch(0.7 0.01 106)",
    label: "Pipeline",
    icon: Workflow,
  },
};

export function getCategoryStyle(category: string): CategoryStyle {
  return CATEGORIES[category as ToolCategory] ?? CATEGORIES.pipeline;
}

export function getCategoryColor(category: string): string {
  return getCategoryStyle(category).color;
}

export const ALL_CATEGORIES = Object.entries(CATEGORIES).map(
  ([id, style]) => ({ id: id as ToolCategory, ...style }),
);
