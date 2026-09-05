// toContentTree — project bowrain's REST block payload into the run-native
// ContentTree the shared preview kit (`@neokapi/ui-primitives/preview`) renders.
//
// The kit speaks the content model: a ContentNode carries typed `source` /
// `targets` run sequences, per-locale `targetMeta`, and run-anchored
// `overlays`. Bowrain's REST surfaces carry the same content plus a flat
// evidence layer alongside it — term matches and entities positioned by byte
// offset into the block's plain source text, voice and rule-based findings already anchored
// to a Anchor, and check results with no position at all. This module is the
// one place those are reconciled, so a rendering surface consumes a ContentTree
// and never re-derives positions itself.
//
// What the web payload cannot fill, this omits rather than invents — the kit
// renders every optional field conditionally:
//   - `targetMeta.score` / `.origin`: the blocks payload carries text + status
//     only. Provenance lives on the separate block-history endpoint and scores
//     on the brand-score endpoint, neither keyed into this projection.
//   - `structure` / `geometry` / `timing` / `media` / `segments` / `relations`:
//     never serialized by the REST blocks payload.
//   - `render`: the generative render AST is produced by the engine
//     (core/projection), not by bowrain's REST server, so the kit falls back to
//     its data-driven `treeToRenderDoc` layout.

import type { CheckFinding, Run } from "@neokapi/contract-types";
import type {
  AnnotationView,
  ContentNode,
  ContentStats,
  ContentTree,
  OverlaySpan,
  OverlayView,
  Anchor,
  TargetMeta,
} from "@neokapi/ui-primitives/preview";
import { rangeAnchorForBytes, textForBytes } from "@neokapi/ui-primitives/preview";
import { codedToRuns } from "@neokapi/ui-primitives";
import { getTargetStatus, getTargetText } from "../components/editor/blockStatus";
import type { BlockInfo, BlockTermMatch, EntityInfo, CheckIssue, SpanInfo } from "../types/api";

/**
 * A run-anchored voice or rule-based finding, as `core/check.Finding`
 * serializes it: the shape the voice-vocabulary check, the /check endpoint, the
 * stored voice scores and the review context's judgement all emit. `position`
 * is already run-anchored, so it needs no offset conversion, and it is optional
 * here because the voice UI's `VoiceFinding` leaves it so.
 *
 * `side` is bowrain's own annotation, naming which text the finding was raised
 * against.
 */
export interface BlockFinding extends Omit<CheckFinding, "position"> {
  position?: Anchor;
  /**
   * The side the finding was raised on — "source" or a target locale key.
   * Defaults to "source".
   */
  side?: string;
}

/** The evidence a review surface has loaded for one block. */
export interface BlockEvidence {
  /** Terminology hits over the source text (byte offsets, server-reported). */
  terms?: BlockTermMatch[];
  /** Voice and rule-based findings carrying their own run range. */
  findings?: BlockFinding[];
  /**
   * Check results from the `qa-check` endpoints. One that carries a position
   * becomes a span like any other finding; one that does not becomes a
   * block-level annotation — a span the payload does not locate must not be
   * guessed at.
   */
  issues?: CheckIssue[];
  /** The locale `issues` were raised on, recorded on each annotation. */
  issueLocale?: string;
}

/** Options shared by the block and tree projections. */
export interface BlockProjectionOptions {
  /** Restrict the projected targets to these locales (default: all present). */
  locales?: string[];
  /** The document's source locale, when the caller knows it. */
  sourceLocale?: string;
}

export interface BlockNodeOptions extends BlockProjectionOptions {
  /** Evidence to layer over this block. */
  evidence?: BlockEvidence;
  /** The id of the containing layer node. */
  parentId?: string;
}

export interface ContentTreeOptions extends BlockProjectionOptions {
  /** The document format id ("json", "markdown", …) — drives the kit's layout. */
  format?: string;
  /** The item name. Present, the blocks are wrapped in one layer node. */
  name?: string;
  /** Evidence keyed by block id. */
  evidence?: Record<string, BlockEvidence>;
}

/**
 * A run sequence from coded text: the flattened `(codedText, spans)` form the
 * cell renderers consume, read back through the same lossless bridge that
 * produced it.
 *
 * Stripping the private-use markers instead is content loss with no error: a
 * unit whose source is "First line<marker>Second line" reads as
 * "First lineSecond line", which is how a review pane came to show
 * "Première ligneDeuxième ligne". A target carries the source's spans, the way
 * every other target renderer reads it (see GridTargetRenderer).
 */
function codedRuns(coded: string | undefined, spans: SpanInfo[] | undefined): Run[] | null {
  if (!coded) return null;
  const runs = codedToRuns(coded, spans ?? []) as Run[];
  return runs.length > 0 ? runs : null;
}

/**
 * A private-use marker with no span to resolve it stands for content this
 * payload cannot describe. It becomes a placeholder run carrying no type, so
 * the preview draws a chip where the code was rather than closing the text over
 * it.
 */
function markersToRuns(text: string): Run[] {
  const out: Run[] = [];
  let buffer = "";
  for (const ch of text) {
    if (ch >= "\uE001" && ch <= "\uE003") {
      if (buffer) {
        out.push({ text: buffer });
        buffer = "";
      }
      out.push({ ph: { id: "", type: "" } } as Run);
      continue;
    }
    buffer += ch;
  }
  if (buffer) out.push({ text: buffer });
  return out;
}

/** The typed source runs, else the coded source, else its plain text. */
function sourceRuns(block: BlockInfo): Run[] {
  if (block.source_runs && block.source_runs.length > 0) return block.source_runs;
  const coded = codedRuns(block.source_coded, block.source_spans);
  if (coded) return coded;
  return markersToRuns(block.source ?? "");
}

/** The typed target runs for a locale, else its coded form, else its plain text. */
function targetRuns(block: BlockInfo, locale: string): Run[] {
  const runs = block.targets_runs?.[locale];
  if (runs && runs.length > 0) return runs;
  const coded = codedRuns(block.targets_coded?.[locale], block.source_spans);
  if (coded) return coded;
  return markersToRuns(getTargetText(block, locale));
}

/** Every locale the block carries a target for, in payload order. */
function localesOf(block: BlockInfo, only?: string[]): string[] {
  const present = Object.keys(block.targets ?? {});
  if (!only) return present;
  const wanted = new Set(only);
  return present.filter((locale) => wanted.has(locale));
}

/** Drop the empty-string props the kit's tooltips would otherwise render. */
function props(entries: Record<string, string | undefined>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [key, value] of Object.entries(entries)) {
    if (value != null && value !== "") out[key] = value;
  }
  return out;
}

/** One overlay per (type, side), the way the engine emits them. */
function overlay(type: OverlayView["type"], side: string, spans: OverlaySpan[]): OverlayView[] {
  return spans.length > 0 ? [{ type, side, spans }] : [];
}

function termSpans(runs: Run[], matches: BlockTermMatch[]): OverlaySpan[] {
  return matches.map((match, i) => ({
    id: `term:${i}`,
    range: rangeAnchorForBytes(runs, match.start, match.end),
    text: textForBytes(runs, match.start, match.end) || match.source_term,
    props: props({
      term: match.source_term,
      target: match.target_terms?.join(", "),
      domain: match.domain,
      status: match.status,
    }),
  }));
}

function entitySpans(runs: Run[], entities: EntityInfo[]): OverlaySpan[] {
  return entities.map((entity, i) => ({
    id: entity.key || `entity:${i}`,
    range: rangeAnchorForBytes(runs, entity.start, entity.end),
    text: textForBytes(runs, entity.start, entity.end) || entity.text,
    props: props({
      type: entity.type,
      dnt: entity.dnt ? "true" : undefined,
      source: entity.source,
    }),
  }));
}

const ZERO_RANGE: Anchor = { kind: "range", start: { run: 0 }, end: { run: 0 } };

function findingSpan(finding: BlockFinding, index: number): OverlaySpan {
  return {
    id: `finding:${index}`,
    range: finding.position ?? ZERO_RANGE,
    text: finding.original_text,
    props: props({
      category: finding.category,
      severity: finding.severity,
      message: finding.message,
      suggestion: finding.suggestion,
      replacement: finding.metadata?.replacement,
      concept_id: finding.metadata?.concept_id,
    }),
  };
}

/**
 * A check issue is a `core/check.Finding` under the Problems panel's names —
 * `type` is the finding's category, and the two-valued severity is the
 * collapsed one that endpoint has always reported. A positioned issue is
 * therefore a span like any other finding's.
 */
function issueSpan(issue: CheckIssue, index: number): OverlaySpan {
  return {
    id: `issue:${index}`,
    range: issue.position ?? ZERO_RANGE,
    text: issue.original_text,
    props: props({
      category: issue.type,
      severity: issue.severity,
      message: issue.message,
      suggestion: issue.suggestion,
    }),
  };
}

/**
 * Findings and positioned check issues become `qa` overlays grouped by side,
 * carrying their category in span props — the anchoring the preview kit's
 * `overlayHighlight` reads to give a voice-vocabulary violation its own accent.
 *
 * One overlay per side, not one per source of evidence: a block whose voice
 * check and rule-based check both flagged the same target must not produce two `qa`
 * overlays for that locale.
 */
function checkOverlays(
  findings: BlockFinding[],
  issues: CheckIssue[],
  issueLocale?: string,
): OverlayView[] {
  const bySide = new Map<string, OverlaySpan[]>();
  const push = (side: string, span: OverlaySpan) => {
    const spans = bySide.get(side) ?? [];
    spans.push(span);
    bySide.set(side, spans);
  };
  findings.forEach((finding, i) => push(finding.side ?? "source", findingSpan(finding, i)));
  issues.forEach((issue, i) => {
    if (issue.position) push(issueLocale ?? "source", issueSpan(issue, i));
  });
  return [...bySide].flatMap(([side, spans]) => overlay("qa", side, spans));
}

/**
 * Check results the payload does not locate, recorded on the block rather than
 * a span — a position that was never reported must not be guessed at.
 */
function issueAnnotations(issues: CheckIssue[], locale?: string): AnnotationView[] {
  return issues
    .filter((issue) => !issue.position)
    .map((issue, i) => ({
      key: `qa:${i}`,
      type: "qa",
      summary: issue.message,
      fields: props({ check: issue.type, severity: issue.severity, locale }),
    }));
}

/**
 * Project one block into a ContentNode: its source and target run sequences,
 * per-locale status, and the evidence layered over it as run-anchored overlays.
 */
export function blockToContentNode(block: BlockInfo, opts: BlockNodeOptions = {}): ContentNode {
  const { evidence = {}, locales, sourceLocale, parentId } = opts;
  const source = sourceRuns(block);

  const targets: Record<string, Run[]> = {};
  const targetMeta: Record<string, TargetMeta> = {};
  for (const locale of localesOf(block, locales)) {
    targets[locale] = targetRuns(block, locale);
    const status = getTargetStatus(block, locale);
    if (status) targetMeta[locale] = { status };
  }

  const overlays: OverlayView[] = [
    ...overlay("term", "source", termSpans(source, evidence.terms ?? [])),
    ...overlay("entity", "source", entitySpans(source, block.entities ?? [])),
    ...checkOverlays(evidence.findings ?? [], evidence.issues ?? [], evidence.issueLocale),
  ];
  const annotations = issueAnnotations(evidence.issues ?? [], evidence.issueLocale);

  const node: ContentNode = {
    kind: "block",
    id: block.id,
    translatable: block.translatable,
    source,
  };
  // The reader's name for the block. For a catalog format it is the unit's key
  // path, which is what the keyed preview groups and labels rows by.
  if (block.name) node.name = block.name;
  if (parentId) node.parentId = parentId;
  if (sourceLocale) node.sourceLocale = sourceLocale;
  if (block.properties && Object.keys(block.properties).length > 0) {
    node.properties = block.properties;
    if (block.properties.type) node.type = block.properties.type;
  }
  if (Object.keys(targets).length > 0) node.targets = targets;
  if (Object.keys(targetMeta).length > 0) node.targetMeta = targetMeta;
  if (overlays.length > 0) node.overlays = overlays;
  if (annotations.length > 0) node.annotations = annotations;
  return node;
}

function statsFor(nodes: ContentNode[], layers: number): ContentStats {
  let runs = 0;
  for (const node of nodes) {
    runs += node.source?.length ?? 0;
    for (const target of Object.values(node.targets ?? {})) runs += target.length;
  }
  return { layers, groups: 0, blocks: nodes.length, data: 0, media: 0, runs };
}

/**
 * Project a list of blocks into a ContentTree. With `name`, the blocks sit
 * under one layer node standing for the item; without it they form the root, so
 * a single-block surface (the focused reviewer) needs no wrapper.
 */
export function blocksToContentTree(
  blocks: BlockInfo[],
  opts: ContentTreeOptions = {},
): ContentTree {
  const { format = "", name, evidence = {}, locales, sourceLocale } = opts;
  const nodes = blocks.map((block) =>
    blockToContentNode(block, {
      evidence: evidence[block.id],
      locales,
      sourceLocale,
      parentId: name,
    }),
  );
  const stats = statsFor(nodes, name ? 1 : 0);
  const root: ContentNode[] = name ? [{ kind: "layer", id: name, name, children: nodes }] : nodes;
  return { format, root, stats };
}
