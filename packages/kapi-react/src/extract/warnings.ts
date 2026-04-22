/**
 * Warnings surfaced during extraction / transform.
 *
 * Both sides of the pipeline (kapi-react's vite plugin and the
 * standalone `kapi-react extract` CLI) walk the same AST. When they
 * decide to translate something the developer didn't explicitly opt
 * into (e.g. a `<div>` with direct text, or an unmapped React
 * component that contains translatable text), they record a warning
 * so the developer sees what's happening and can opt out with
 * `translate="no"` or stabilise hashes with a `componentMap` entry.
 */

export type WarningKind = "unknown-component" | "dyn-label-splice" | "ternary-attr-complex";

export interface Warning {
  kind: WarningKind;
  filename: string;
  line: number;
  /**
   * For `unknown-component`: the source tag (div, section, TabsTrigger, …).
   * For `dyn-label-splice`: the expression source (`meta.label`, `item.title`, …).
   * For `ternary-attr-complex`: the attribute name (title, placeholder, …).
   */
  tag: string;
  /** Short snippet so the developer can locate it visually. */
  snippet: string;
}

export interface WarningCollector {
  add(w: Warning): void;
  list(): Warning[];
}

/** Default collector backed by an in-memory array. */
export function createWarningCollector(): WarningCollector {
  const warnings: Warning[] = [];
  const seen = new Set<string>();
  return {
    add(w) {
      // Dedupe on filename:line:tag so one offending element doesn't
      // fire repeatedly when the plugin re-transforms on save.
      const key = `${w.filename}:${w.line}:${w.tag}:${w.kind}`;
      if (seen.has(key)) return;
      seen.add(key);
      warnings.push(w);
    },
    list: () => warnings.slice(),
  };
}

/** Renders a warning as a short stderr-friendly line. */
export function formatWarning(w: Warning): string {
  const loc = `${w.filename}:${w.line}`;
  switch (w.kind) {
    case "unknown-component":
      return (
        `[neokapi] ${loc}: <${w.tag}> is an unmapped component with ` +
        `translatable text — extracted. Add a componentMap entry to ` +
        `stabilise hashes: { ${w.tag}: '<underlying-html-tag>' }.\n` +
        `  ↳ ${w.snippet}`
      );
    case "dyn-label-splice":
      return (
        `[neokapi] ${loc}: {${w.tag}} is rendered as JSX text — the ` +
        `property name suggests a user-visible string that won't be ` +
        `translated. Extract the label (e.g. t("key") or <T>…</T>) or ` +
        `mark the parent translate="no" if intentional.\n` +
        `  ↳ ${w.snippet}`
      );
    case "ternary-attr-complex":
      return (
        `[neokapi] ${loc}: ${w.tag}={…} uses a conditional whose ` +
        `branches aren't both plain string literals — can't extract. ` +
        `Lift the strings to the branches (cond ? "A" : "B") or use ` +
        `t() explicitly.\n` +
        `  ↳ ${w.snippet}`
      );
  }
}
