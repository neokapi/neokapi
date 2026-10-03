#!/usr/bin/env node
// Headless smoke test for the change contract in the browser engine: boots
// kapi-cli.wasm in Node and drives kapiRead, kapiApply and kapiDescribe
// through the @neokapi/engine facade (makeRuntime), the way a lab does. It
// proves that a read hands out the references and revisions `kapi inspect`
// prints, that an edit built from them lands through the byte-faithful write
// and is recorded in the browser's workspace log, that a stale replay and a
// change set that does not decode come back as refusals, and that the
// actor a call names reaches the policy.
//
//   Run: node --experimental-strip-types scripts/verify-snippets/change-smoke.ts
//   (make change-wasm-smoke builds the engine first)
import { readFileSync } from "node:fs";
import { dirname, join, resolve as pathResolve } from "node:path";
import { fileURLToPath } from "node:url";
import { runInThisContext } from "node:vm";
import type { ReadPage } from "../../packages/contract-types/src/index.ts";
import { createMemFS } from "../../packages/engine/src/memfs.ts";
import { ChangeRefused, makeRuntime } from "../../packages/engine/src/runtime.ts";
import { installSQLiteBridge, loadSQLite } from "../../packages/engine/src/sqlite.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const wasmDir = join(pathResolve(__dirname, "../.."), "web/static/wasm");

const dec = new TextDecoder();
const enc = new TextEncoder();

let captured = "";
const mem = createMemFS({
  onStdout: (c: Uint8Array) => (captured += dec.decode(c)),
  onStderr: (c: Uint8Array) => (captured += dec.decode(c)),
});
const g = globalThis as Record<string, unknown>;
g.fs = mem.fs;
g.process = Object.assign({}, process, mem.process, { env: process.env });
runInThisContext(readFileSync(join(wasmDir, "wasm_exec.js"), "utf8"));
installSQLiteBridge(await loadSQLite());
const go = new (g.Go as new () => {
  env: Record<string, string>;
  importObject: WebAssembly.Imports;
  run(i: WebAssembly.Instance): Promise<void>;
})();
go.env = { CLICOLOR_FORCE: "0" };
const ready = new Promise<void>((res) => (g.__kapiCliReady = res));
const { instance } = await WebAssembly.instantiate(
  readFileSync(join(wasmDir, "kapi-cli.wasm")),
  go.importObject,
);
void go.run(instance);
await ready;
const rt = makeRuntime(mem);

let failures = 0;
const ok = (label: string, cond: boolean, detail = "") => {
  console.log(`${cond ? "✓" : "✗"} ${label}${detail ? "  " + detail : ""}`);
  if (!cond) failures++;
};
const read = (path: string) => dec.decode(mem.vol.readFile(path));

// ── 0. The ABI names the entry points and stays at version 1 ─────────────────
const abi = (g.kapiEngineABI as () => { abi: number; functions: string[] })();
ok("the engine ABI stays at version 1", abi.abi === 1, `abi=${abi.abi}`);
for (const name of ["kapiRead", "kapiApply", "kapiDescribe", "kapiRun"]) {
  ok(`kapiEngineABI lists ${name}`, Array.from(abi.functions).includes(name));
}

// ── 1. A project whose page has a link, and its translations ─────────────────
const PAGE = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Guide</title></head>
<body>
<p>Read the <a href="https://old.example/guide">shop guide</a> before you <b>order</b>.</p>
</body></html>
`;
mem.vol.mkdirp("/site/docs");
mem.vol.writeFile(
  "/site/kapi.yaml",
  enc.encode(
    'version: v1\nname: site\ndefaults:\n  source_language: en\n  target_languages: [fr]\ncollections:\n  - name: docs\n    content:\n      - path: "docs/*.html"\n        target: "i18n/{lang}/{path}.html"\n',
  ),
);
mem.vol.writeFile("/site/docs/page.html", enc.encode(PAGE));
const project = { project: "/site" };

// ── 2. A read hands out references and revisions, as kapi inspect does ──────
const page: ReadPage = await rt.read({ doc: "docs/page.html" }, project);
const para = page.blocks.find((b) => b.text.includes("shop guide"));
ok("kapiRead reads the page's blocks", page.format === "html" && !!para, JSON.stringify(page).slice(0, 160));
ok("a read lists the link's code and its writable href", para?.codes?.["1"]?.attrs?.href === "https://old.example/guide");
captured = "";
const code = await rt.run(["inspect", "/site/docs/page.html", "--jsonl", "-p", "/site/kapi.yaml"]);
const inspected = captured
  .split("\n")
  .filter((l) => l.trim().startsWith("{"))
  .map((l) => JSON.parse(l) as { ref?: { block?: string }; rev?: string })
  .find((r) => r.ref?.block === para?.ref.block);
ok(
  "kapi inspect prints the reference and revision kapiRead returned",
  code === 0 && inspected?.rev === para?.rev,
  `inspect=${inspected?.rev} read=${para?.rev}`,
);

// ── 3. What the format supports ──────────────────────────────────────────────
const described = await rt.describe({ format: "html" });
ok(
  "kapiDescribe names the operations HTML supports",
  described.format === "html" && described.ops.set_content !== null && described.ops.replace_text !== null,
  JSON.stringify(described.ops).slice(0, 160),
);
const byDoc = await rt.describe({ doc: "docs/page.html" }, project);
ok("kapiDescribe describes a document's format", byDoc.format === "html");

// ── 4. An edit lands, is recorded, and a replay is stale ─────────────────────
const set = {
  note: "Rename the guide",
  ops: [
    {
      op: "replace_text" as const,
      at: para!.ref,
      if_match: para!.rev,
      edits: [{ find: "shop guide", text: "handbook" }],
    },
  ],
};
const applied = await rt.apply(set, project);
ok("kapiApply applies the change set", applied.status === "applied", JSON.stringify(applied).slice(0, 200));
ok("the edit is recorded in the workspace log", typeof applied.record === "string" && applied.record !== "", `record=${applied.record}`);
ok(
  "only the link's text changed, byte for byte",
  read("/site/docs/page.html") === PAGE.replace("shop guide", "handbook"),
  read("/site/docs/page.html").split("\n")[3],
);
const replay = await rt.apply(set, project);
ok(
  "a replay is refused as stale with the current text",
  replay.status === "refused" &&
    replay.ops[0]?.error?.code === "stale" &&
    (replay.ops[0]?.current?.text ?? "").includes("handbook"),
  JSON.stringify(replay.ops[0]).slice(0, 200),
);

// ── 5. A translation's basis comes back from the block history ──────────────
const now = await rt.read({ doc: "docs/page.html" }, project);
const source = now.blocks.find((b) => b.text.includes("handbook"))!;
const fr = await rt.apply(
  {
    ops: [
      {
        op: "set_content",
        at: { ...source.ref, edition: "fr" },
        if_match: "absent",
        text: 'Lisez le <x id="1"/>manuel<x id="/1"/> avant de <x id="2"/>commander<x id="/2"/>.',
      },
    ],
  },
  project,
);
ok("a translation is written to its own file", fr.status === "applied", JSON.stringify(fr.ops[0]));
const withFr = await rt.read({ doc: "docs/page.html", editions: ["fr"] }, project);
const frEdition = withFr.blocks.find((b) => b.ref.block === source.ref.block)?.editions?.["fr"];
ok(
  "a read shows the basis the browser's block history recorded",
  frEdition?.basis === source.rev && frEdition?.stale === false,
  JSON.stringify(frEdition),
);

// ── 6. Refusals ──────────────────────────────────────────────────────────────
const undecodable = await rt.apply(
  JSON.parse('{"ops":[{"op":"set_content","at":{"doc":"docs/page.html","block":"p"}}]}'),
  project,
);
ok(
  "a change set that does not decode is a refused result naming the field",
  undecodable.status === "refused" && undecodable.error?.code === "invalid" && !!undecodable.error?.pointer,
  JSON.stringify(undecodable.error),
);
const missing = await rt.read({ doc: "docs/missing.html" }, project).catch((e: unknown) => e);
ok(
  "a read of a document the project does not hold throws the refusal",
  missing instanceof ChangeRefused && missing.result.error?.code === "not_found",
  String(missing),
);
const agentTerm = await rt.apply(
  { ops: [{ op: "term", action: "upsert", term: "handbook", locale: "en" }] },
  { ...project, actor: { kind: "agent", name: "lab-agent" } },
);
ok(
  "an agent's term is refused: writing a term is a person's",
  agentTerm.status === "refused" && agentTerm.ops[0]?.error?.code === "not_permitted",
  JSON.stringify(agentTerm.ops[0]?.error),
);

// ── 7. Outside a project: a path under the working directory, no record ─────
mem.vol.mkdirp("/scratch");
mem.vol.writeFile("/scratch/app.json", enc.encode('{"title": "Welcome"}\n'));
mem.process.chdir("/scratch");
const loose = await rt.read({ doc: "app.json" });
const title = loose.blocks[0];
const looseApplied = await rt.apply({
  ops: [{ op: "set_content", at: title.ref, if_match: title.rev, text: "Welcome back" }],
});
ok(
  "outside a project an edit lands and nothing is recorded",
  looseApplied.status === "applied" && looseApplied.record === null && read("/scratch/app.json") === '{"title": "Welcome back"}\n',
  JSON.stringify(looseApplied).slice(0, 160),
);

// ── 8. A command's flags stay with the command ──────────────────────────────
// The engine has one App: a command run with --encoding must not leave a
// later call writing a UTF-8 file back as Windows-1252.
mem.vol.writeFile("/scratch/cafe.md", enc.encode("Café au lait.\n"));
await rt.run(["inspect", "/scratch/cafe.md", "--encoding", "windows-1252"]);
const cafe = (await rt.read({ doc: "cafe.md" })).blocks[0];
const cafeApplied = await rt.apply({
  ops: [{ op: "set_content", at: cafe.ref, if_match: cafe.rev, text: "Café crème." }],
});
ok(
  "a call after a command run with --encoding writes the file as UTF-8",
  cafeApplied.status === "applied" && read("/scratch/cafe.md") === "Café crème.\n",
  JSON.stringify(read("/scratch/cafe.md")),
);

// ── 9. A bilingual catalog is read in the language the call names ───────────
// A PO catalog's msgstr is a translation only in a language its reader is
// told: the read's editions and the apply's operations name it.
mem.vol.writeFile(
  "/scratch/messages.po",
  enc.encode('msgid ""\nmsgstr ""\n"Content-Type: text/plain; charset=UTF-8\\n"\n\nmsgid "Hello"\nmsgstr ""\n'),
);
const po = (await rt.read({ doc: "messages.po", editions: ["fr"] })).blocks[0];
const poApplied = await rt.apply({
  ops: [
    {
      op: "set_content",
      at: { doc: "messages.po", block: po.ref.block, edition: "fr" },
      if_match: "absent",
      text: "Bonjour",
    },
  ],
});
const poBack = (await rt.read({ doc: "messages.po", editions: ["fr"] })).blocks[0];
ok(
  "a call translates a PO catalog in place and reads the msgstr back",
  poApplied.status === "applied" &&
    read("/scratch/messages.po").includes('msgid "Hello"\nmsgstr "Bonjour"\n') &&
    poBack.editions?.fr?.text === "Bonjour",
  JSON.stringify(poApplied.ops[0]?.error ?? poBack.editions),
);

// ── 10. Commands and calls take turns ───────────────────────────────────────
// A command started beside a call reconfigures the engine; the engine runs
// them one at a time, so both land whichever starts first.
mem.vol.writeFile("/scratch/two.json", enc.encode('{"a": "One", "b": "Two"}\n'));
const two = await rt.read({ doc: "two.json" });
const [blockA, blockB] = two.blocks;
mem.vol.writeFile(
  "/scratch/edit-a.json",
  enc.encode(JSON.stringify({ ops: [{ op: "set_content", at: blockA.ref, if_match: blockA.rev, text: "Uno" }] })),
);
const [cmdCode, callResult] = await Promise.all([
  rt.run(["apply", "/scratch/edit-a.json"]),
  rt.apply({ ops: [{ op: "set_content", at: blockB.ref, if_match: blockB.rev, text: "Dos" }] }),
]);
ok(
  "a command and a call started together both land",
  cmdCode === 0 && callResult.status === "applied" && read("/scratch/two.json") === '{"a": "Uno", "b": "Dos"}\n',
  JSON.stringify({ cmdCode, status: callResult.status, file: read("/scratch/two.json") }),
);

console.log(failures === 0 ? "\nALL CHANGE CONTRACT CHECKS PASSED" : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
