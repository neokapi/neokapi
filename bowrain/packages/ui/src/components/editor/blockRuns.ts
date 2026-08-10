import type { Run as SchemaRun } from "@neokapi/kapi-format";
import type { Run } from "@neokapi/contract-types";
import { runsToCoded } from "@neokapi/ui-primitives";
import type { BlockInfo, SpanInfo } from "../../types/api";

/**
 * Read-boundary normalisation of a block payload into the coded-text + spans
 * shape the render primitives (`FormattedSourceDisplay`, `SourceCellDisplay`,
 * `CollapsedTargetCell`, `InlineCodeEditor`) consume.
 *
 * The bowrain REST server ships blocks as RFC 0001 typed `Run[]`
 * (`source_runs` / `targets_runs` + `has_inline_codes`), but every render
 * primitive reads `source_coded` / `source_spans` / `targets_coded` /
 * `has_spans`. Nothing converted between the two, so on the live path inline
 * codes silently fell back to plain text in *both* the editor and review
 * surfaces. Converting once here — via the same lossless `runsToCoded` bridge
 * the write path already uses — makes source and target render faithfully and
 * identically everywhere, with no second rendering path.
 */

/** The raw block shape as the server serialises it (superset of BlockInfo). */
export interface ServerBlockInfo extends BlockInfo {
  /** Server flag mirroring the frontend's `has_spans`. */
  has_inline_codes?: boolean;
}

/**
 * Coded text + spans for a run sequence, or null when it can't be flattened.
 *
 * The payload's runs are the generated contract shape (@neokapi/contract-types,
 * from Go); `runsToCoded` reads the hand-authored schema shape
 * (@neokapi/kapi-format). The two agree on every field the bridge touches and
 * differ only in `RunConstraints`, whose flags the generated type marks
 * optional (the Go tags are omitempty) and the schema type requires — and which
 * the bridge never reads.
 */
function codeRuns(runs: Run[] | undefined): { codedText: string; spans: SpanInfo[] } | null {
  if (!runs || runs.length === 0) return null;
  try {
    return runsToCoded(runs as SchemaRun[]);
  } catch {
    // Plural / select wrappers can't flatten to a single coded string; the
    // plain `source` / `targets[locale].text` (ICU) is rendered instead.
    return null;
  }
}

/**
 * Populate `source_coded` / `source_spans` / `targets_coded` / `has_spans` from
 * the server's typed runs when the coded fields are absent. Idempotent: a block
 * that already carries coded text (Storybook fixtures, or a future server that
 * sends coded directly) passes through untouched.
 *
 * The typed runs ride along on the normalised block. Flattening to coded text
 * loses the run boundaries that overlay ranges, segment spans and check-finding
 * positions anchor to, so a run-native consumer — `preview/toContentTree`,
 * feeding the shared preview kit — needs the sequences themselves, not their
 * projection.
 */
export function normalizeServerBlock(raw: ServerBlockInfo): BlockInfo {
  const { has_inline_codes, ...block } = raw;
  const { source_runs, targets_runs } = block;

  let sourceCoded = block.source_coded;
  let sourceSpans = block.source_spans;
  if (sourceCoded == null && source_runs) {
    const coded = codeRuns(source_runs);
    if (coded) {
      sourceCoded = coded.codedText;
      sourceSpans = coded.spans;
    }
  }

  let targetsCoded = block.targets_coded;
  if (targetsCoded == null && targets_runs) {
    const map: Record<string, string> = {};
    for (const [locale, runs] of Object.entries(targets_runs)) {
      const coded = codeRuns(runs);
      if (coded) map[locale] = coded.codedText;
    }
    if (Object.keys(map).length > 0) targetsCoded = map;
  }

  const hasSpans =
    block.has_spans || has_inline_codes === true || (sourceSpans != null && sourceSpans.length > 0);

  return {
    ...block,
    source_coded: sourceCoded,
    source_spans: sourceSpans,
    targets_coded: targetsCoded,
    has_spans: hasSpans,
  };
}

/** Normalise a list of server blocks (see {@link normalizeServerBlock}). */
export function normalizeServerBlocks(raw: ServerBlockInfo[]): BlockInfo[] {
  return raw.map(normalizeServerBlock);
}
