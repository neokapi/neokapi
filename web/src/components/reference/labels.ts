// The words a reader sees for the enums the reference dataset carries. Held at
// module level so the build-time transform rewrites each `t()` call; inside a
// text-bearing element it would not.

import { t } from "@neokapi/i18n-react/runtime";
import type { ReferenceSource } from "@neokapi/reference-data";

// A tool's category (core/schema.NormalizeCategory).
const CATEGORY_LABELS: Record<string, string> = {
  "text-processing": t("Text processing", "tool category"),
  quality: t("Quality", "tool category"),
  translation: t("Translation", "tool category"),
  analysis: t("Analysis", "tool category"),
  other: t("Other", "tool category"),
};

export function categoryLabel(id: string | undefined): string {
  if (!id) return CATEGORY_LABELS.other;
  return CATEGORY_LABELS[id] ?? id;
}

// Where an entry comes from.
export const SOURCE_LABELS: Record<ReferenceSource, string> = {
  "built-in": t("Built-in", "an entry maintained in neokapi itself"),
  plugin: t("Plugin", "an entry an installed plugin provides"),
  okapi: t("Okapi", "an entry the Okapi Framework bridge provides"),
};

export function sourceLabel(source: string): string {
  return SOURCE_LABELS[source as ReferenceSource] ?? source;
}
