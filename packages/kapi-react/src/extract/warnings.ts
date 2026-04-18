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

export type WarningKind = 'unknown-component';

export interface Warning {
  kind: WarningKind;
  filename: string;
  line: number;
  /** The source tag as written (div, section, TabsTrigger, …). */
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
  return (
    `[neokapi] ${loc}: <${w.tag}> is an unmapped component with ` +
    `translatable text — extracted. Add a componentMap entry to ` +
    `stabilise hashes: { ${w.tag}: '<underlying-html-tag>' }.\n` +
    `  ↳ ${w.snippet}`
  );
}
