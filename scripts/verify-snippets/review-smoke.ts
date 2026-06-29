#!/usr/bin/env node
// Headless smoke test for the review→approve loop in WASM: boots kapi-cli.wasm
// in Node, seeds a fully-translated kapi project (recipe + source + target), and
// exercises the project state-store commands that the regular walkthrough smoke
// harness can't reach (it seeds only single fixtures, never a multi-file
// project). Proves `kapi status`, `kapi status --review`, and `kapi apply`
// (kind:"review") run against the in-memory filesystem — the prerequisite for an
// in-browser review walkthrough. Mirrors the boot logic in lab-smoke.ts.
//   Run: node --experimental-strip-types scripts/verify-snippets/review-smoke.ts
import { readFileSync } from "node:fs";
import { resolve as pathResolve, join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { runInThisContext } from "node:vm";
import { createMemFS } from "./memfs.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = pathResolve(__dirname, "../..");
const wasmDir = join(REPO_ROOT, "web/static/wasm");

const dec = new TextDecoder();
const enc = new TextEncoder();

// Capture wasm stdout per command so we can assert on `status` output; stderr
// streams through to the host for debugging.
let out = "";
const mem = createMemFS({
  onStdout: (c: Uint8Array) => {
    out += dec.decode(c);
  },
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

// A project whose single content file is already fully translated into fr — flat
// target path (no subdirectory) so it seeds cleanly into the in-memory volume.
const RECIPE = `version: v1
name: demo
defaults:
  source_language: en
  target_languages: [fr]
content:
  - path: messages.json
    target: "messages.{lang}.json"
`;
const SOURCE = JSON.stringify(
  { greeting: "Hello, World!", farewell: "See you tomorrow", items: { cart: "Your cart is empty" } },
  null,
  2,
);
const TARGET_FR = JSON.stringify(
  { greeting: "Bonjour le monde !", farewell: "À demain", items: { cart: "Votre panier est vide" } },
  null,
  2,
);
// The approval change-set: bless the `greeting` unit's fr translation.
const REVIEW = JSON.stringify({
  kind: "review",
  file: "messages.fr.json",
  id: "greeting",
  locale: "fr",
  status: "reviewed",
});

mem.vol.writeFile("/project/demo.kapi", enc.encode(RECIPE));
mem.vol.writeFile("/project/messages.json", enc.encode(SOURCE));
mem.vol.writeFile("/project/messages.fr.json", enc.encode(TARGET_FR));
mem.vol.writeFile("/project/review.jsonl", enc.encode(REVIEW));

// Discover the project the way a real session does: from the working directory.
// `kapi apply` resolves the project by an upward walk from cwd (no -p flag), so
// the cwd must be inside the project tree.
try {
  (mem.process as any).chdir?.("/project");
} catch {
  /* ignore */
}

let failures = 0;
const ok = (label: string, cond: boolean, detail = "") => {
  console.log(`${cond ? "✓" : "✗"} ${label}${detail ? "  " + detail : ""}`);
  if (!cond) failures++;
};

const P = "/project/demo.kapi";
const ANSI = /\x1b\[[0-9;]*m/g;
const run = async (argv: string[]): Promise<{ code: number; stdout: string }> => {
  out = "";
  const code: number = await (globalThis as any).kapiRun(argv);
  return { code, stdout: out.replace(ANSI, "") };
};

// ── 1. status: fr is fully translated, nothing reviewed yet ──────────────────
const s1 = await run(["status", "-p", P]);
ok("status exits 0 in wasm", s1.code === 0, `code=${s1.code}`);
ok("status reports fr translated 100%", /\bfr\b[\s\S]*100%/.test(s1.stdout), s1.stdout.trim().split("\n").pop() ?? "");
ok("status shows 0% reviewed before approval", / 0%/.test(s1.stdout));

// ── 2. status --review: the worklist of translated-not-reviewed units ────────
const s2 = await run(["status", "--review", "--json", "-p", P]);
ok("status --review exits 0 in wasm", s2.code === 0, `code=${s2.code}`);
let queue: any = null;
try {
  // The --json payload is the last JSON object on stdout (skip any leading log).
  const start = s2.stdout.indexOf("{");
  queue = start >= 0 ? JSON.parse(s2.stdout.slice(start)) : null;
} catch {
  console.log("   [debug] review stdout:", JSON.stringify(s2.stdout.slice(0, 200)));
}
ok("review queue lists 3 pending units", queue?.pending?.length === 3, `got=${queue?.pending?.length}`);
ok(
  "review queue is addressable by file/key/locale",
  !!queue?.pending?.find((u: any) => u.key === "greeting" && u.locale === "fr" && u.file === "messages.fr.json"),
);

// ── 3. apply kind:"review": the decision lands in the state store ────────────
// apply has no -p flag — it discovers the project from cwd (/project, above).
const s3 = await run(["apply", "review.jsonl"]);
ok("apply review exits 0 in wasm", s3.code === 0, `code=${s3.code}`);
const stateRaw = mem.vol.readFile("/project/.kapi-state.json");
ok("apply wrote the committed state store (.kapi-state.json)", !!stateRaw);
const state = stateRaw ? JSON.parse(dec.decode(stateRaw)) : {};
ok("state store kind is kapi-project-state", state.kind === "kapi-project-state", `kind=${state.kind}`);
ok(
  "state records the reviewed greeting unit with a targetHash",
  !!state.units?.find((u: any) => u.unit === "greeting" && u.status === "reviewed" && u.targetHash),
);

// ── 4. status again: the approval is derived back as reviewed coverage ───────
const s4 = await run(["status", "-p", P]);
ok("status (post-approval) exits 0", s4.code === 0, `code=${s4.code}`);
ok(
  "reviewed coverage climbs to 33% (1 of 3) after one approval",
  / 33%/.test(s4.stdout),
  s4.stdout.trim().split("\n").pop() ?? "",
);

// ── 5. the approved unit leaves the review queue (content-hash bound) ─────────
const s5 = await run(["status", "--review", "--json", "-p", P]);
let queue2: any = null;
try {
  const start = s5.stdout.indexOf("{");
  queue2 = start >= 0 ? JSON.parse(s5.stdout.slice(start)) : null;
} catch {
  /* leave null */
}
ok("review queue drops to 2 after approving one unit", queue2?.pending?.length === 2, `got=${queue2?.pending?.length}`);
ok(
  "the approved greeting unit is no longer in the queue",
  !queue2?.pending?.find((u: any) => u.key === "greeting"),
);

console.log(failures === 0 ? "\nALL REVIEW SMOKE CHECKS PASSED" : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
