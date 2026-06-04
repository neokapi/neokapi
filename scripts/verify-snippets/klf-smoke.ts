#!/usr/bin/env node
// Headless parity smoke for the KLF wasm endpoint: boots kapi-cli.wasm in Node,
// then for the core spec operations compares the canonical Go engine (via the
// `klf` endpoint) against the TypeScript mirror (@neokapi/kapi-format imported
// from source). This is the headless equivalent of the docs KLF Tests page.
// Run: node --experimental-strip-types scripts/verify-snippets/klf-smoke.ts
import { readFileSync } from "node:fs";
import { resolve as pathResolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { runInThisContext } from "node:vm";
import { createHash } from "node:crypto";
import { createMemFS } from "./memfs.ts";
// Relative source imports (not the bare specifier) so --experimental-strip-types
// doesn't have to process the package under node_modules.
import {
  marshalFile,
  renderBlockHtml,
  resolveAnchor,
  validateTargetAgainstSource,
} from "../../packages/kapi-format/src/index.ts";
import type { Block, File, Run } from "../../packages/kapi-format/src/block.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = pathResolve(__dirname, "../..");
const wasmDir = join(REPO_ROOT, "web/docs/static/wasm");

const dec = new TextDecoder();
const mem = createMemFS({
  onStdout: (c: Uint8Array) => process.stderr.write(dec.decode(c)),
  onStderr: (c: Uint8Array) => process.stderr.write(dec.decode(c)),
});
(globalThis as any).fs = mem.fs;
(globalThis as any).process = Object.assign({}, process, mem.process, { env: process.env });

runInThisContext(readFileSync(join(wasmDir, "wasm_exec.js"), "utf8"));
const Go = (globalThis as any).Go;
const go = new Go();
go.env = { CLICOLOR_FORCE: "0" };
const ready = new Promise<void>((res) => ((globalThis as any).__kapiCliReady = res));
const { instance } = await WebAssembly.instantiate(
  readFileSync(join(wasmDir, "kapi-cli.wasm")),
  go.importObject,
);
void go.run(instance);
await ready;

const klf = (req: unknown): any => JSON.parse((globalThis as any).klf(JSON.stringify(req)) as string);
const sha = (b: Uint8Array): string => createHash("sha256").update(b).digest("hex");

let failures = 0;
const ok = (label: string, cond: boolean, detail = "") => {
  console.log(`${cond ? "✓" : "✗"} ${label}${detail ? "  " + detail : ""}`);
  if (!cond) failures++;
};

// ── fixtures ──────────────────────────────────────────────────────────────
const filesHeading: Block = {
  id: "files-heading",
  hash: "2xykvb",
  translatable: true,
  type: "jsx:element",
  source: [
    { text: "Files " },
    { pcOpen: { id: "1", type: "jsx:element", subType: "span", data: '<span className="muted">', equiv: "muted", disp: "span" } },
    { text: "(" },
    { ph: { id: "2", type: "jsx:var", subType: "number", data: "{count}", equiv: "count", disp: "count" } },
    { text: " matched)" },
    { pcClose: { id: "1", type: "jsx:element", subType: "span", data: "</span>", equiv: "muted" } },
  ],
  placeholders: [
    { name: "muted", kind: "element", jsType: "ReactNode", sourceExpr: '<span className="muted">...</span>' },
    { name: "count", kind: "variable", jsType: "number", sourceExpr: "count" },
  ],
  properties: { file: "src/FilesHeading.tsx", line: 4, component: "FilesHeading", jsxPath: "FilesHeading > h2", element: "h2" },
};
const shoppingCart: Block = {
  id: "shopping-cart-plural",
  hash: "9QpZ11",
  translatable: true,
  type: "jsx:element",
  source: [
    {
      plural: {
        pivot: "count",
        forms: {
          one: [{ text: "1 item in your cart" }],
          other: [
            { ph: { id: "1", type: "jsx:var", subType: "number", data: "{count}", equiv: "count", disp: "count" } },
            { text: " items in your cart" },
          ],
          zero: [{ text: "Your cart is empty" }],
        },
      },
    },
  ],
  placeholders: [{ name: "count", kind: "icu-pivot", jsType: "number", sourceExpr: "items" }],
  properties: { file: "src/ShoppingCart.tsx", line: 4, component: "ShoppingCart", jsxPath: "ShoppingCart > p > Plural", element: "Plural" },
};
const file: File = {
  schemaVersion: "1.0",
  kind: "kapi-localization-format",
  created: "2026-04-15T10:00:00Z",
  generator: { id: "@neokapi/kapi-format-examples", version: "0.0.1", capabilities: ["extract", "preview"] },
  project: { id: "neokapi-kapi-format-examples", sourceLocale: "en" },
  documents: [{ id: "examples", documentType: "jsx", path: "examples/all.tsx", blocks: [filesHeading, shoppingCart] }],
};

// ── 1. Serialization parity (Go sha == TS sha) ─────────────────────────────
const goRound = klf({ op: "roundtrip", klf: `${JSON.stringify(file, null, 2)}\n` });
ok("roundtrip ok", goRound.ok === true, goRound.error ?? "");
const tsSha = sha(marshalFile(file));
ok("Go canonical sha == TS canonical sha", goRound.sha256 === tsSha, `go=${String(goRound.sha256).slice(0, 12)} ts=${tsSha.slice(0, 12)}`);

// Edge cases that previously diverged Go↔TS (see /klf-tests conformance):
// an empty placeholders array (must serialize as `[]`, not be omitted) and a
// multi-key preview.sampleValues (keys must sort to match Go map ordering),
// plus a sub run.
const edge: File = {
  schemaVersion: "1.0",
  kind: "kapi-localization-format",
  generator: { id: "edge", version: "1" },
  project: { id: "edge", sourceLocale: "en" },
  documents: [
    {
      id: "d",
      documentType: "jsx",
      path: "edge.tsx",
      blocks: [
        {
          id: "outer",
          hash: "h",
          translatable: true,
          type: "jsx:element",
          source: [{ text: "Hi " }, { sub: { id: "1", ref: "inner", equiv: "cta" } }],
          placeholders: [{ name: "cta", kind: "node", sourceExpr: "<X/>" }],
          properties: { file: "edge.tsx", line: 1, component: "E", jsxPath: "p", element: "p" },
          preview: { storyId: "e--default", sampleValues: { label: "react", index: 3, deletable: true } },
        },
        {
          id: "inner",
          hash: "h2",
          translatable: true,
          type: "jsx:element",
          source: [{ text: "Confirm" }],
          placeholders: [],
          properties: { file: "edge.tsx", line: 2, component: "E", jsxPath: "a", element: "a" },
        },
      ],
    },
  ],
};
const goEdge = klf({ op: "roundtrip", klf: `${JSON.stringify(edge, null, 2)}\n` });
ok("edge roundtrip ok", goEdge.ok === true, goEdge.error ?? "");
const tsEdge = sha(marshalFile(edge));
ok("Go sha == TS sha (empty placeholders + sampleValues order + sub)", goEdge.sha256 === tsEdge, `go=${String(goEdge.sha256).slice(0, 12)} ts=${tsEdge.slice(0, 12)}`);

// ── 2. HTML preview parity ─────────────────────────────────────────────────
for (const b of [filesHeading, shoppingCart]) {
  const goHtml = klf({ op: "renderHtml", block: b }).html;
  const tsHtml = renderBlockHtml(b);
  ok(`renderHtml parity: ${b.id}`, goHtml === tsHtml, goHtml === tsHtml ? "" : `\n   go=${goHtml}\n   ts=${tsHtml}`);
}

// ── 3. Anchor resolution parity ────────────────────────────────────────────
const anchors = [
  { kind: "run", block: "files-heading", path: [3], runId: "2" },
  { kind: "range", block: "files-heading", path: [4], offset: 1, length: 7 },
  { kind: "form", block: "shopping-cart-plural", path: [0], key: "one" },
  { kind: "run", block: "files-heading", path: [3], runId: "99" },
] as const;
const normTs = (r: any): string =>
  !r.ok ? `fail:${r.reason}` : r.kind === "run" ? `ok:run:${r.run.ph?.id ?? r.run.pcOpen?.id ?? r.run.sub?.id}` : r.kind === "range" ? `ok:range:${r.offset}+${r.length}` : r.kind === "form" ? `ok:form:${r.runs.length}` : "ok:block";
const normGo = (r: any): string =>
  !r.ok ? `fail:${r.reason}` : r.kind === "run" ? `ok:run:${r.runId}` : r.kind === "range" ? `ok:range:${r.rangeOffset}+${r.rangeLength}` : r.kind === "form" ? `ok:form:${r.formRunCount}` : "ok:block";
for (const a of anchors) {
  const b = a.block === "files-heading" ? filesHeading : shoppingCart;
  const goRes = normGo(klf({ op: "resolveAnchor", block: b, anchor: a }).resolution);
  const tsRes = normTs(resolveAnchor(b, a as any));
  ok(`anchor parity: ${a.kind} ${JSON.stringify(a.path)} runId=${(a as any).runId ?? "-"}`, goRes === tsRes, `go=${goRes} ts=${tsRes}`);
}

// ── 4. Target validation parity (missing + valid) ──────────────────────────
const validTarget: Run[] = [
  { pcOpen: { id: "1", type: "jsx:element", subType: "span", data: '<span className="muted">', equiv: "muted" } },
  { ph: { id: "2", type: "jsx:var", subType: "number", data: "{count}", equiv: "count" } },
  { pcClose: { id: "1", type: "jsx:element", subType: "span", data: "</span>", equiv: "muted" } },
];
const missingTarget: Run[] = [
  { pcOpen: { id: "1", type: "jsx:element", subType: "span", data: '<span className="muted">', equiv: "muted" } },
  { pcClose: { id: "1", type: "jsx:element", subType: "span", data: "</span>", equiv: "muted" } },
];
const normKinds = (ks: string[]) => (ks.length === 0 ? "valid" : [...ks].sort().join(","));
for (const [name, t] of [["valid", validTarget], ["missing", missingTarget]] as const) {
  const goK = normKinds((klf({ op: "validateTarget", source: filesHeading, target: t }).errors ?? []).map((e: any) => e.kind));
  const tsK = normKinds(validateTargetAgainstSource(filesHeading, t).map((e) => e.kind));
  ok(`target validation parity: ${name}`, goK === tsK, `go=${goK} ts=${tsK}`);
}

// ── 5. Structural validation (canonical Go) ────────────────────────────────
const unclosed: Block = {
  id: "unclosed",
  hash: "x",
  translatable: true,
  type: "jsx:element",
  source: [{ pcOpen: { id: "1", type: "jsx:element", subType: "b", data: "<b>", equiv: "b" } }, { text: "bold" }],
  placeholders: [{ name: "b", kind: "element", sourceExpr: "<b>" }],
  properties: { file: "x", line: 1, component: "X", jsxPath: "X", element: "p" },
};
const ucErrs = (klf({ op: "validateBlock", block: unclosed }).errors ?? []).map((e: any) => e.kind);
ok("validateBlock flags unclosed-paired-code", ucErrs.includes("unclosed-paired-code"), JSON.stringify(ucErrs));

console.log(failures === 0 ? "\nALL KLF PARITY CHECKS PASSED" : `\n${failures} KLF CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
