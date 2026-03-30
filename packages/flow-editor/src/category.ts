// Category visual identity — color-coded rails for each tool type.

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
    color: "oklch(0.65 0.19 252)",
    bg: "oklch(0.65 0.19 252 / 0.12)",
    text: "oklch(0.75 0.14 252)",
    label: "Translate",
    icon: Languages,
  },
  validate: {
    color: "oklch(0.75 0.15 75)",
    bg: "oklch(0.75 0.15 75 / 0.12)",
    text: "oklch(0.82 0.12 75)",
    label: "Validate",
    icon: ShieldCheck,
  },
  transform: {
    color: "oklch(0.65 0.18 300)",
    bg: "oklch(0.65 0.18 300 / 0.12)",
    text: "oklch(0.78 0.14 300)",
    label: "Transform",
    icon: Wand2,
  },
  convert: {
    color: "oklch(0.7 0.14 180)",
    bg: "oklch(0.7 0.14 180 / 0.12)",
    text: "oklch(0.8 0.1 180)",
    label: "Convert",
    icon: RefreshCw,
  },
  enrich: {
    color: "oklch(0.7 0.17 145)",
    bg: "oklch(0.7 0.17 145 / 0.12)",
    text: "oklch(0.8 0.13 145)",
    label: "Enrich",
    icon: Sparkles,
  },
  pipeline: {
    color: "oklch(0.6 0.02 260)",
    bg: "oklch(0.6 0.02 260 / 0.12)",
    text: "oklch(0.7 0.01 260)",
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
