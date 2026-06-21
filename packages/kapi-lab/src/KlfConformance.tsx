import React, { useEffect, useState } from "react";
import {
  marshalFile,
  renderBlockHtml,
  resolveAnchor,
  validateTargetAgainstSource,
} from "@neokapi/kapi-format";
import type { AnnotationAnchor, Block, File, Run } from "@neokapi/kapi-format";
import { useLabRuntime } from "./useLabRuntime";
import GateOverlay from "./GateOverlay";
import { useRunGate } from "./useRunGate";
import type { LabRuntime, LabRuntimeAssets } from "./useLabRuntime";
import {
  emailBody,
  filesHeading,
  klfText,
  klfSampleById,
  likeNotification,
  shoppingCart,
  tagChip,
} from "./klfFixtures";
import styles from "./Klf.module.css";

export interface KlfConformanceProps {
  /** WASM asset URLs from the host; null defers booting (e.g. during SSR). */
  assets: LabRuntimeAssets | null;
}

type Category = "serialization" | "preview" | "anchor" | "target" | "structure";

interface ConfCase {
  id: string;
  name: string;
  description: string;
  category: Category;
  /** Expected normalized value; when set, each engine is checked against it. */
  expected?: string;
  /** Run against the canonical Go engine (via the wasm `klf` endpoint). */
  runGo: (rt: LabRuntime) => Promise<string> | string;
  /** Run against the TypeScript mirror; omit for canonical-engine-only cases. */
  runTs?: () => Promise<string> | string;
}

interface CaseResult {
  c: ConfCase;
  goValue: string;
  tsValue: string | null;
  goPass: boolean;
  tsPass: boolean | null;
  agree: boolean | null;
}

// KlfConformance executes the KLF spec conformance suite in the browser against
// BOTH implementations: the canonical Go engine (core/klf, compiled to wasm)
// and the TypeScript mirror (@neokapi/kapi-format). For the operations both
// implement — deterministic serialization, Level-1 HTML preview, annotation
// anchor resolution, and required-placeholder target validation — it asserts
// the two engines agree. The structural/envelope cases run against the
// canonical Go engine only (the TypeScript mirror does not expose an identical
// API surface for those), and are labelled accordingly.
export default function KlfConformance({ assets }: KlfConformanceProps): React.ReactElement {
  const runtime = useLabRuntime(assets, { autoBoot: false });
  const gate = useRunGate(runtime);
  const [results, setResults] = useState<CaseResult[] | null>(null);
  const [running, setRunning] = useState(false);

  useEffect(() => {
    if (!runtime.ready) return;
    let cancelled = false;
    setRunning(true);
    void (async () => {
      const out: CaseResult[] = [];
      for (const c of CASES) {
        const goValue = await c.runGo(runtime);
        const tsValue = c.runTs ? await c.runTs() : null;
        const goPass =
          c.expected != null
            ? goValue === c.expected
            : tsValue != null
              ? goValue === tsValue
              : false;
        const tsPass =
          tsValue == null
            ? null
            : c.expected != null
              ? tsValue === c.expected
              : tsValue === goValue;
        const agree = tsValue == null ? null : goValue === tsValue;
        out.push({ c, goValue, tsValue, goPass, tsPass, agree });
      }
      if (!cancelled) {
        setResults(out);
        setRunning(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runtime.ready, runtime]);

  const total = results?.length ?? CASES.length;
  const goPassed = results?.filter((r) => r.goPass).length ?? 0;
  const dual = results?.filter((r) => r.agree != null) ?? [];
  const agreed = dual.filter((r) => r.agree).length;

  return (
    <div className={`kapi-reference relative ${styles.lab}`}>
      <div className={styles.summary}>
        {runtime.status === "booting" && (
          <span className={styles.status}>Booting the kapi engine…</span>
        )}
        {runtime.status === "error" && (
          <span className={`${styles.status} ${styles.statusError}`}>
            Failed to start: {runtime.error}
          </span>
        )}
        {runtime.ready && (
          <>
            <span className={styles.summaryStat}>
              <span
                className={`${styles.summaryCount} ${goPassed === total ? styles.pass : styles.fail}`}
              >
                {goPassed}/{total}
              </span>
              cases pass
            </span>
            <span className={styles.summaryStat}>
              <span
                className={`${styles.summaryCount} ${agreed === dual.length ? styles.pass : styles.fail}`}
              >
                {agreed}/{dual.length}
              </span>
              dual-engine cases agree
            </span>
            {running && <span className={styles.status}>running…</span>}
          </>
        )}
      </div>

      <div className="min-h-[420px]">
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Case</th>
              <th>Category</th>
              <th>Go</th>
              <th>TypeScript</th>
              <th>Agree</th>
              <th>Result</th>
            </tr>
          </thead>
          <tbody>
            {(results ?? []).map((r) => (
              <tr key={r.c.id}>
                <td>
                  <div className={styles.caseName}>{r.c.name}</div>
                  <div className={styles.caseDesc}>{r.c.description}</div>
                  <div className={styles.detail}>{r.goValue}</div>
                </td>
                <td>
                  <span className={styles.catTag}>{r.c.category}</span>
                </td>
                <td>
                  <span className={`${styles.verdict} ${r.goPass ? styles.pass : styles.fail}`}>
                    {r.goPass ? "✓ pass" : "✗ fail"}
                  </span>
                </td>
                <td>
                  {r.tsPass == null ? (
                    <span className={`${styles.verdict} ${styles.na}`}>— canonical only</span>
                  ) : (
                    <span className={`${styles.verdict} ${r.tsPass ? styles.pass : styles.fail}`}>
                      {r.tsPass ? "✓ pass" : "✗ fail"}
                    </span>
                  )}
                </td>
                <td>
                  {r.agree == null ? (
                    <span className={`${styles.verdict} ${styles.na}`}>—</span>
                  ) : (
                    <span className={`${styles.verdict} ${r.agree ? styles.pass : styles.fail}`}>
                      {r.agree ? "✓" : "✗"}
                    </span>
                  )}
                </td>
                <td className={styles.detail}>
                  {r.c.expected != null ? `expected: ${r.c.expected}` : "parity"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <GateOverlay
        gate={gate}
        title="KLF conformance"
        description="Run the KLF conformance suite in your browser."
      />
    </div>
  );
}

// ─── normalization helpers ───────────────────────────────────────────────

async function sha256Hex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes as unknown as ArrayBuffer);
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function goAnchor(rt: LabRuntime, block: Block, anchor: AnnotationAnchor): string {
  const res = rt.klf({ op: "resolveAnchor", block, anchor });
  const r = (res.resolution as Record<string, unknown>) ?? {};
  if (!r.ok) return `fail:${String(r.reason)}`;
  switch (r.kind) {
    case "run":
      return `ok:run:${String(r.runId)}`;
    case "range":
      return `ok:range:${String(r.rangeOffset)}+${String(r.rangeLength)}`;
    case "form":
      return `ok:form:${String(r.formRunCount)}`;
    default:
      return "ok:block";
  }
}

function tsAnchor(block: Block, anchor: AnnotationAnchor): string {
  const r = resolveAnchor(block, anchor);
  if (!r.ok) return `fail:${r.reason}`;
  switch (r.kind) {
    case "run": {
      const run = r.run as Run;
      const id =
        "ph" in run ? run.ph.id : "pcOpen" in run ? run.pcOpen.id : "sub" in run ? run.sub.id : "";
      return `ok:run:${id}`;
    }
    case "range":
      return `ok:range:${r.offset}+${r.length}`;
    case "form":
      return `ok:form:${r.runs.length}`;
    default:
      return "ok:block";
  }
}

function goValidateTarget(rt: LabRuntime, source: Block, target: Run[]): string {
  const res = rt.klf({ op: "validateTarget", source, target });
  const errs = (res.errors as Array<{ kind: string }>) ?? [];
  return normKinds(errs.map((e) => e.kind));
}

function tsValidateTarget(source: Block, target: Run[]): string {
  return normKinds(validateTargetAgainstSource(source, target).map((e) => e.kind));
}

function goValidateBlock(rt: LabRuntime, block: unknown): string {
  const res = rt.klf({ op: "validateBlock", block });
  if (!res.ok) return "decode-rejected";
  const errs = (res.errors as Array<{ kind: string }>) ?? [];
  return normKinds(errs.map((e) => e.kind));
}

function goAccepts(rt: LabRuntime, klf: string): string {
  return rt.klf({ op: "roundtrip", klf }).ok ? "accepted" : "rejected";
}

function normKinds(kinds: string[]): string {
  return kinds.length === 0 ? "valid" : [...kinds].sort().join(",");
}

// ─── fixtures derived for cases ──────────────────────────────────────────

const fullFile: File = klfSampleById("full").file;
const selectFile: File = klfSampleById("select").file;
const subFile: File = klfSampleById("sub").file;
const pluralFile: File = klfSampleById("plural").file;

// A valid files-heading target: preserves the `muted` paired code and the
// `count` variable, only the literal text is translated.
const filesHeadingValidTarget: Run[] = [
  {
    pcOpen: {
      id: "1",
      type: "jsx:element",
      subType: "span",
      data: '<span className="muted">',
      equiv: "muted",
    },
  },
  { text: "(" },
  { ph: { id: "2", type: "jsx:var", subType: "number", data: "{count}", equiv: "count" } },
  { text: " trouvés)" },
  { pcClose: { id: "1", type: "jsx:element", subType: "span", data: "</span>", equiv: "muted" } },
];
// Drops the required `count` variable.
const filesHeadingMissingTarget: Run[] = [
  {
    pcOpen: {
      id: "1",
      type: "jsx:element",
      subType: "span",
      data: '<span className="muted">',
      equiv: "muted",
    },
  },
  { text: "(aucun)" },
  { pcClose: { id: "1", type: "jsx:element", subType: "span", data: "</span>", equiv: "muted" } },
];
// Introduces a placeholder the source never declared.
const filesHeadingExtraTarget: Run[] = [
  ...filesHeadingValidTarget,
  { ph: { id: "9", type: "jsx:var", data: "{stray}", equiv: "stray" } },
];
// A tag-chip target that keeps only the required `label`, dropping both
// optional nodes — legitimate.
const tagChipOptionalTarget: Run[] = [
  { ph: { id: "2", type: "jsx:var", subType: "string", data: "{label}", equiv: "label" } },
];

// A block with a run carrying two discriminators — the wire decoder must
// reject it (validated structurally only when assembled in memory).
const malformedBlock = {
  id: "bad",
  hash: "x",
  translatable: true,
  type: "jsx:element",
  source: [{ text: "a", ph: { id: "1", type: "jsx:var", data: "{x}", equiv: "x" } }],
  placeholders: [],
  properties: { file: "x", line: 1, component: "X", jsxPath: "X", element: "p" },
};

// An unclosed paired code (pcOpen with no pcClose).
const unclosedBlock: Block = {
  id: "unclosed",
  hash: "x",
  translatable: true,
  type: "jsx:element",
  source: [
    { pcOpen: { id: "1", type: "jsx:element", subType: "b", data: "<b>", equiv: "b" } },
    { text: "bold" },
  ],
  placeholders: [{ name: "b", kind: "element", sourceExpr: "<b>" }],
  properties: { file: "x", line: 1, component: "X", jsxPath: "X", element: "p" },
};
const unmatchedCloseBlock: Block = {
  id: "unmatched",
  hash: "x",
  translatable: true,
  type: "jsx:element",
  source: [{ pcClose: { id: "1", type: "jsx:element", subType: "b", data: "</b>" } }],
  placeholders: [],
  properties: { file: "x", line: 1, component: "X", jsxPath: "X", element: "p" },
};
const unknownPlaceholderBlock: Block = {
  id: "unknown-ph",
  hash: "x",
  translatable: true,
  type: "jsx:element",
  source: [{ ph: { id: "1", type: "jsx:var", data: "{x}", equiv: "undeclared" } }],
  placeholders: [],
  properties: { file: "x", line: 1, component: "X", jsxPath: "X", element: "p" },
};

function badEnvelope(mutate: (f: File) => void): string {
  const clone = JSON.parse(klfText(klfSampleById("files-heading").file)) as File;
  mutate(clone);
  return klfText(clone);
}

const CASES: ConfCase[] = [
  // ── Serialization (deterministic byte parity) ──
  {
    id: "ser-full",
    name: "Round-trip the complete document",
    description: "Decode → re-marshal; both engines emit identical canonical bytes.",
    category: "serialization",
    runGo: (rt) => rt.klf({ op: "roundtrip", klf: klfText(fullFile) }).sha256 as string,
    runTs: () => sha256Hex(marshalFile(fullFile)),
  },
  {
    id: "ser-plural",
    name: "Round-trip a plural block",
    description: "Plural form keys serialize in the same (sorted) order in both engines.",
    category: "serialization",
    runGo: (rt) => rt.klf({ op: "roundtrip", klf: klfText(pluralFile) }).sha256 as string,
    runTs: () => sha256Hex(marshalFile(pluralFile)),
  },
  {
    id: "ser-select",
    name: "Round-trip a select block",
    description: "Select case keys serialize identically across implementations.",
    category: "serialization",
    runGo: (rt) => rt.klf({ op: "roundtrip", klf: klfText(selectFile) }).sha256 as string,
    runTs: () => sha256Hex(marshalFile(selectFile)),
  },
  {
    id: "ser-sub",
    name: "Round-trip a subblock reference",
    description: "A document with a sub run and its referenced block round-trips byte-identically.",
    category: "serialization",
    runGo: (rt) => rt.klf({ op: "roundtrip", klf: klfText(subFile) }).sha256 as string,
    runTs: () => sha256Hex(marshalFile(subFile)),
  },

  // ── Preview (Level-1 HTML parity) ──
  {
    id: "prev-files",
    name: "Render files-heading to HTML",
    description: "Paired code + variable render to the same <kat-block> HTML.",
    category: "preview",
    runGo: (rt) => rt.klf({ op: "renderHtml", block: filesHeading }).html as string,
    runTs: () => renderBlockHtml(filesHeading),
  },
  {
    id: "prev-tag",
    name: "Render tag-chip to HTML",
    description: "Conditional jsx:node placeholders render identically.",
    category: "preview",
    runGo: (rt) => rt.klf({ op: "renderHtml", block: tagChip }).html as string,
    runTs: () => renderBlockHtml(tagChip),
  },
  {
    id: "prev-plural",
    name: "Render a plural block to HTML",
    description: "Plural forms render in the same order with the same labels.",
    category: "preview",
    runGo: (rt) => rt.klf({ op: "renderHtml", block: shoppingCart }).html as string,
    runTs: () => renderBlockHtml(shoppingCart),
  },
  {
    id: "prev-select",
    name: "Render a select block to HTML",
    description: "Select cases render identically (other sorts last).",
    category: "preview",
    runGo: (rt) => rt.klf({ op: "renderHtml", block: likeNotification }).html as string,
    runTs: () => renderBlockHtml(likeNotification),
  },
  {
    id: "prev-sub",
    name: "Render a sub run to HTML",
    description: "A subblock reference renders to the same neokapi-sub span.",
    category: "preview",
    runGo: (rt) => rt.klf({ op: "renderHtml", block: emailBody }).html as string,
    runTs: () => renderBlockHtml(emailBody),
  },

  // ── Anchor resolution ──
  {
    id: "anc-block",
    name: "Resolve a block anchor",
    description: "A block-kind anchor resolves to the whole block.",
    category: "anchor",
    expected: "ok:block",
    runGo: (rt) => goAnchor(rt, filesHeading, { kind: "block", block: "files-heading" }),
    runTs: () => tsAnchor(filesHeading, { kind: "block", block: "files-heading" }),
  },
  {
    id: "anc-run",
    name: "Resolve a run anchor",
    description: "A run anchor resolves to the run at the path with the matching id.",
    category: "anchor",
    expected: "ok:run:2",
    runGo: (rt) => goAnchor(rt, tagChip, { kind: "run", block: "tag-chip", path: [2], runId: "2" }),
    runTs: () => tsAnchor(tagChip, { kind: "run", block: "tag-chip", path: [2], runId: "2" }),
  },
  {
    id: "anc-range",
    name: "Resolve a range anchor",
    description: "A character range inside a text run resolves to the offset/length.",
    category: "anchor",
    expected: "ok:range:1+7",
    runGo: (rt) =>
      goAnchor(rt, filesHeading, {
        kind: "range",
        block: "files-heading",
        path: [4],
        offset: 1,
        length: 7,
      }),
    runTs: () =>
      tsAnchor(filesHeading, {
        kind: "range",
        block: "files-heading",
        path: [4],
        offset: 1,
        length: 7,
      }),
  },
  {
    id: "anc-form",
    name: "Resolve a form anchor",
    description: "A plural-form anchor resolves to the runs of that form.",
    category: "anchor",
    expected: "ok:form:1",
    runGo: (rt) =>
      goAnchor(rt, shoppingCart, {
        kind: "form",
        block: "shopping-cart-plural",
        path: [0],
        key: "one",
      }),
    runTs: () =>
      tsAnchor(shoppingCart, {
        kind: "form",
        block: "shopping-cart-plural",
        path: [0],
        key: "one",
      }),
  },
  {
    id: "anc-runid",
    name: "Detect a stale run id",
    description: "A run anchor whose recorded id no longer matches fails as run-id-mismatch.",
    category: "anchor",
    expected: "fail:run-id-mismatch",
    runGo: (rt) =>
      goAnchor(rt, tagChip, { kind: "run", block: "tag-chip", path: [2], runId: "99" }),
    runTs: () => tsAnchor(tagChip, { kind: "run", block: "tag-chip", path: [2], runId: "99" }),
  },
  {
    id: "anc-oob",
    name: "Detect an out-of-bounds path",
    description: "A path step past the end of the runs fails as path-out-of-bounds.",
    category: "anchor",
    expected: "fail:path-out-of-bounds",
    runGo: (rt) =>
      goAnchor(rt, filesHeading, { kind: "run", block: "files-heading", path: [99], runId: "1" }),
    runTs: () =>
      tsAnchor(filesHeading, { kind: "run", block: "files-heading", path: [99], runId: "1" }),
  },
  {
    id: "anc-block-nf",
    name: "Detect a block mismatch",
    description: "An anchor for a different block id fails as block-not-found.",
    category: "anchor",
    expected: "fail:block-not-found",
    runGo: (rt) => goAnchor(rt, filesHeading, { kind: "block", block: "nope" }),
    runTs: () => tsAnchor(filesHeading, { kind: "block", block: "nope" }),
  },
  {
    id: "anc-form-nf",
    name: "Detect a missing plural form",
    description: "A form anchor for a non-existent form fails as form-not-found.",
    category: "anchor",
    expected: "fail:form-not-found",
    runGo: (rt) =>
      goAnchor(rt, shoppingCart, {
        kind: "form",
        block: "shopping-cart-plural",
        path: [0],
        key: "many",
      }),
    runTs: () =>
      tsAnchor(shoppingCart, {
        kind: "form",
        block: "shopping-cart-plural",
        path: [0],
        key: "many",
      }),
  },

  // ── Target validation (required-placeholder preservation) ──
  {
    id: "tgt-valid",
    name: "Accept a faithful target",
    description: "A target preserving every required placeholder is valid in both engines.",
    category: "target",
    expected: "valid",
    runGo: (rt) => goValidateTarget(rt, filesHeading, filesHeadingValidTarget),
    runTs: () => tsValidateTarget(filesHeading, filesHeadingValidTarget),
  },
  {
    id: "tgt-missing",
    name: "Flag a dropped required placeholder",
    description: "Dropping a required placeholder is missing-placeholder in both engines.",
    category: "target",
    expected: "missing-placeholder",
    runGo: (rt) => goValidateTarget(rt, filesHeading, filesHeadingMissingTarget),
    runTs: () => tsValidateTarget(filesHeading, filesHeadingMissingTarget),
  },
  {
    id: "tgt-optional",
    name: "Allow dropping optional placeholders",
    description: "Optional jsx:node placeholders may be dropped — still valid in both engines.",
    category: "target",
    expected: "valid",
    runGo: (rt) => goValidateTarget(rt, tagChip, tagChipOptionalTarget),
    runTs: () => tsValidateTarget(tagChip, tagChipOptionalTarget),
  },
  {
    id: "tgt-extra",
    name: "Flag an invented placeholder",
    description:
      "The canonical Go engine also flags a placeholder the source never declared (extra-placeholder); the TypeScript mirror checks only required-placeholder preservation.",
    category: "target",
    expected: "extra-placeholder",
    runGo: (rt) => goValidateTarget(rt, filesHeading, filesHeadingExtraTarget),
    // No runTs — the mirror does not implement the extra-placeholder check.
  },

  // ── Structure & envelope (canonical Go engine) ──
  {
    id: "str-valid",
    name: "Accept a well-formed block",
    description: "A balanced block with declared placeholders validates clean.",
    category: "structure",
    expected: "valid",
    runGo: (rt) => goValidateBlock(rt, filesHeading),
  },
  {
    id: "str-unclosed",
    name: "Flag an unclosed paired code",
    description: "A pcOpen with no matching pcClose is unclosed-paired-code.",
    category: "structure",
    expected: "unclosed-paired-code",
    runGo: (rt) => goValidateBlock(rt, unclosedBlock),
  },
  {
    id: "str-unmatched",
    name: "Flag an unmatched close code",
    description: "A pcClose with no matching pcOpen is unmatched-close-code.",
    category: "structure",
    expected: "unmatched-close-code",
    runGo: (rt) => goValidateBlock(rt, unmatchedCloseBlock),
  },
  {
    id: "str-unknown-ph",
    name: "Flag an undeclared placeholder",
    description: "A run referencing a placeholder absent from the list is unknown-placeholder.",
    category: "structure",
    expected: "unknown-placeholder",
    runGo: (rt) => goValidateBlock(rt, unknownPlaceholderBlock),
  },
  {
    id: "str-malformed",
    name: "Reject a malformed run on the wire",
    description: "A run object with two discriminators is rejected by the decoder.",
    category: "structure",
    expected: "decode-rejected",
    runGo: (rt) => goValidateBlock(rt, malformedBlock),
  },
  {
    id: "env-kind",
    name: "Reject an unknown envelope kind",
    description: "A file whose kind is not kapi-localization-format is rejected.",
    category: "structure",
    expected: "rejected",
    runGo: (rt) =>
      goAccepts(
        rt,
        badEnvelope((f) => ((f as { kind: string }).kind = "something-else")),
      ),
  },
  {
    id: "env-major",
    name: "Reject an unknown major version",
    description: "A file with an unrecognized major schemaVersion is rejected.",
    category: "structure",
    expected: "rejected",
    runGo: (rt) =>
      goAccepts(
        rt,
        badEnvelope((f) => ((f as { schemaVersion: string }).schemaVersion = "2.0")),
      ),
  },
];
