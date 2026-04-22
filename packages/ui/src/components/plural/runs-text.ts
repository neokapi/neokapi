/**
 * Minimal, reversible conversion between a `Run[]` sequence and
 * the plain-text shape a translator edits in a textarea.
 *
 *   TextRun          →  verbatim characters
 *   PlaceholderRun   →  `{equiv}` token (falls back to id)
 *   PcOpen/PcClose   →  `{=equiv}` / `{/=equiv}` markers so paired
 *                       codes survive a round-trip
 *   SubRun           →  `[equiv]` as a marker
 *   PluralRun        →  flattened via ICU-like text; reversed parse
 *                       isn't supported (the component keeps the
 *                       structure alongside instead)
 *
 * The parse direction uses the Block's `placeholders` table as the
 * authority on a token's typed metadata — so `{count}` in the
 * translator's input becomes a `PlaceholderRun` with the original
 * source's type / subType / data / equiv intact.
 */

import type { Placeholder, Run } from "@neokapi/kapi-format";

/**
 * Render a Run[] as plain text with `{equiv}` tokens.
 * Non-text runs contribute exactly one token; their order is
 * preserved so a textarea edit can reintroduce them.
 */
export function runsToText(runs: readonly Run[]): string {
  let out = "";
  for (const r of runs) {
    if ("text" in r) out += r.text;
    else if ("ph" in r) out += `{${r.ph.equiv || r.ph.id}}`;
    else if ("pcOpen" in r) out += `{=${r.pcOpen.equiv || r.pcOpen.id}}`;
    else if ("pcClose" in r) out += `{/=${r.pcClose.equiv || r.pcClose.id}}`;
    else if ("sub" in r) out += `[${r.sub.equiv || r.sub.id}]`;
    else if ("plural" in r) {
      // Plural inside a flat target isn't representable in the
      // textarea; callers should switch to per-form editing.
      out += `{${r.plural.pivot}, plural, ...}`;
    } else if ("select" in r) {
      out += `{${r.select.pivot}, select, ...}`;
    }
  }
  return out;
}

/**
 * Parse a translator-edited string into a Run[] using the Block's
 * placeholders as the authority. Unknown `{token}` references are
 * preserved as text rather than silently dropped — the editor
 * surfaces them to the translator as a warning.
 */
export function textToRuns(
  text: string,
  placeholders: readonly Placeholder[],
  sourceRuns: readonly Run[] = [],
): Run[] {
  const byEquiv = indexByEquiv(sourceRuns);
  const byName = new Map<string, Placeholder>();
  for (const p of placeholders) byName.set(p.name, p);

  const out: Run[] = [];
  let buffer = "";
  const flush = () => {
    if (buffer.length > 0) {
      out.push({ text: buffer });
      buffer = "";
    }
  };

  let i = 0;
  while (i < text.length) {
    if (text[i] !== "{") {
      buffer += text[i];
      i++;
      continue;
    }
    const end = text.indexOf("}", i);
    if (end < 0) {
      buffer += text.slice(i);
      break;
    }
    const inner = text.slice(i + 1, end);
    const run = resolveToken(inner, byEquiv, byName);
    if (run) {
      flush();
      out.push(run);
    } else {
      // Unknown token — keep as literal so the translator sees it.
      buffer += text.slice(i, end + 1);
    }
    i = end + 1;
  }
  flush();
  return out;
}

// ─── Internals ───────────────────────────────────────────────────

interface IndexedRun {
  kind: "ph" | "pcOpen" | "pcClose" | "sub";
  run: Run;
}

function indexByEquiv(runs: readonly Run[]): Map<string, IndexedRun> {
  const out = new Map<string, IndexedRun>();
  for (const r of runs) {
    if ("ph" in r && r.ph.equiv) out.set(r.ph.equiv, { kind: "ph", run: r });
    else if ("pcOpen" in r && r.pcOpen.equiv) out.set(r.pcOpen.equiv, { kind: "pcOpen", run: r });
    else if ("pcClose" in r && r.pcClose.equiv)
      out.set(`/${r.pcClose.equiv}`, { kind: "pcClose", run: r });
    else if ("sub" in r && r.sub.equiv) out.set(r.sub.equiv, { kind: "sub", run: r });
  }
  return out;
}

function resolveToken(
  inner: string,
  byEquiv: Map<string, IndexedRun>,
  byName: Map<string, Placeholder>,
): Run | null {
  // Paired-code markers from runsToText: `=equiv` for open, `/=equiv` for close.
  if (inner.startsWith("/=")) {
    const equiv = inner.slice(2);
    const hit = byEquiv.get(`/${equiv}`);
    if (hit && hit.kind === "pcClose") return hit.run;
  }
  if (inner.startsWith("=")) {
    const equiv = inner.slice(1);
    const hit = byEquiv.get(equiv);
    if (hit && hit.kind === "pcOpen") return hit.run;
  }

  // Plain `{name}` tokens: look up by source ph first (keeps
  // structured metadata), fall back to the placeholder table so
  // we still produce a typed run when the translator introduces a
  // placeholder the source doesn't expose yet.
  const fromEquiv = byEquiv.get(inner);
  if (fromEquiv && fromEquiv.kind === "ph") return fromEquiv.run;

  const meta = byName.get(inner);
  if (meta) {
    return {
      ph: {
        id: inner,
        type:
          meta.kind === "variable" ? "jsx:var" : meta.kind === "node" ? "jsx:node" : "jsx:element",
        data: `{${inner}}`,
        equiv: inner,
        ...(meta.jsType ? { subType: String(meta.jsType) } : {}),
      },
    } as Run;
  }

  return null;
}
