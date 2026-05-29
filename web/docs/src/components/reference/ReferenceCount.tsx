import { formats, tools } from "@neokapi/reference-data";

type Kind = "format" | "tool";

interface ReferenceCountProps {
  /** Which dataset to count. */
  kind: Kind;
  /**
   * When set, count only entries from this engine ("built-in" or "okapi").
   * Omit to count the whole dataset (built-in + okapi-bridge).
   */
  source?: "built-in" | "okapi";
  /**
   * When true, render the count rounded down to the nearest ten with a "+"
   * suffix (e.g. 106 → "100+"), matching the restrained "NN+" phrasing used in
   * prose. When false (default) the exact count is rendered.
   */
  round?: boolean;
}

/**
 * ReferenceCount renders the number of formats or tools in the generated
 * reference dataset, so prose never hardcodes a count that the code controls.
 * The dataset is produced by `make generate-reference-docs` (scripts/gen-refs)
 * and shipped via @neokapi/reference-data; this number tracks it automatically.
 */
export default function ReferenceCount({ kind, source, round = false }: ReferenceCountProps) {
  const dataset = kind === "format" ? formats : tools;
  const entries = source ? dataset.entries.filter((e) => e.source === source) : dataset.entries;
  const n = entries.length;
  const label = round ? `${Math.floor(n / 10) * 10}+` : String(n);
  return <>{label}</>;
}
