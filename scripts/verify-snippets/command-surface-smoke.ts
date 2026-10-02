#!/usr/bin/env node
// Headless smoke test for the browser command surface: boots kapi-cli.wasm in
// Node and proves that every verb the lab can reach either runs or explains
// itself — never `unknown command`.
//
// This is the runtime half of the guard. The compile-time half is the Go test
// cli.TestBrowserCommandSurface, which pins cli.BrowserCommandSet against
// cli.KapiCommandSet so a verb cannot go missing from the browser build in the
// first place. This script checks the other failure mode: a verb that is
// registered but broken, and a lab component calling a verb by a spelling the
// CLI no longer has — which is exactly how /lab/convert came to answer every
// conversion with `unknown command "convert" for "kapi"` after #1180 renamed
// the toolbox proxies to their standalone binary names.
//
//   Run: node --experimental-strip-types scripts/verify-snippets/command-surface-smoke.ts
import { readFileSync, readdirSync } from "node:fs";
import { resolve as pathResolve, join, dirname } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { runInThisContext } from "node:vm";
import { createMemFS } from "./memfs.ts";
import { installSQLiteBridge, loadSQLite } from "../../packages/engine/src/sqlite.ts";
import { LOOSE_SAMPLES } from "../../packages/kapi-playground/src/samples.ts";
import { CLI_EXAMPLES } from "../../packages/kapi-playground/src/cliExamples.ts";
import { parseCommand } from "../../packages/kapi-playground/src/argv.ts";
import { getFixture } from "../../packages/kapi-playground/src/fixtures.ts";
import { SAMPLES } from "../../packages/kapi-lab/src/samples.ts";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = pathResolve(__dirname, "../..");
const wasmDir = join(REPO_ROOT, "web/static/wasm");

const dec = new TextDecoder();
const enc = new TextEncoder();

let captured = "";
const mem = createMemFS({
  onStdout: (c: Uint8Array) => {
    captured += dec.decode(c);
  },
  onStderr: (c: Uint8Array) => {
    captured += dec.decode(c);
  },
});
(globalThis as any).fs = mem.fs;
(globalThis as any).process = Object.assign({}, process, mem.process, { env: process.env });

runInThisContext(readFileSync(join(wasmDir, "wasm_exec.js"), "utf8"));
// The engine's stores run on SQLite through the bridge @neokapi/engine
// installs on a page before Go starts.
installSQLiteBridge(await loadSQLite());
const Go = (globalThis as any).Go;
const go = new Go();
// Mirror the browser exactly: @neokapi/engine boots with CLICOLOR_FORCE=1 so
// the playground terminal shows kapi's real styling. Capturing output therefore
// has to strip ANSI the way useLabRuntime.runCapture does — a smoke that turned
// colour off would not catch a lab component parsing coloured JSON.
go.env = { CLICOLOR_FORCE: "1", HOME: "/home", XDG_CACHE_HOME: "/cache" };
const ready = new Promise<void>((res) => ((globalThis as any).__kapiCliReady = res));
const { instance } = await WebAssembly.instantiate(
  readFileSync(join(wasmDir, "kapi-cli.wasm")),
  go.importObject,
);
void go.run(instance);
await ready;

// eslint-disable-next-line no-control-regex
const ANSI_ESCAPE = /\x1b\[[0-9;]*m/g;

async function run(...argv: string[]): Promise<{ code: number; out: string }> {
  captured = "";
  const code: number = await (globalThis as any).kapiRun(argv);
  return { code, out: captured.replace(ANSI_ESCAPE, "") };
}

let failures = 0;
function ok(label: string, cond: boolean, detail = ""): void {
  if (!cond) failures++;
  console.log(`${cond ? "  ok" : "FAIL"}: ${label}${detail ? `  — ${detail}` : ""}`);
}

// ── 1. No reachable verb answers "unknown command" ─────────────────────────
//
// The visible surface comes from the binary itself (cobra's completion
// endpoint), so this sweep widens automatically as commands are added. Hidden
// verbs are not completable, so the ones a user or a lab component types by
// name are listed explicitly; cli.TestBrowserCommandSurface is what guarantees
// the set is complete.
const HIDDEN_VERBS = ["kgrep", "ksed", "kcat", "kconv", "kdiff", "hook", "engine"];

console.log("command-surface-smoke: every reachable verb resolves");
const completion = await run("__complete", "");
ok("`__complete` enumerates the browser surface", completion.code === 0, `exit ${completion.code}`);
const completed = completion.out
  .split("\n")
  .map((l) => l.split("\t")[0].trim())
  .filter((l) => l && !l.startsWith(":") && !l.startsWith("Completion ended"));
ok("completion returned a plausible surface", completed.length >= 20, `${completed.length} verbs`);

const verbs = [...new Set([...completed, ...HIDDEN_VERBS])].sort();
for (const verb of verbs) {
  const r = await run(verb, "--help");
  ok(
    `\`kapi ${verb} --help\` resolves`,
    r.code === 0 && !/unknown command/.test(r.out),
    r.code === 0 ? "" : r.out.trim().split("\n")[0],
  );
}

// ── 2. Verbs the browser cannot run explain themselves ─────────────────────
//
// Every entry here is a cli.browserGaps key. The message must name the verb and
// the facility the browser denies — "unknown command" would tell a lab user the
// verb does not exist, which is false and unactionable.
console.log("command-surface-smoke: browser-unavailable verbs are honest");
const GAP_VERBS = ["plugin", "models", "credentials", "telemetry", "update", "mcp", "engine"];
for (const verb of GAP_VERBS) {
  // Bare, and with the subcommand a user would reach for — neither may fall
  // through to a second "unknown command" from an unregistered subcommand.
  for (const argv of [[verb], [verb, "list"]]) {
    const r = await run(...argv);
    const honest =
      r.code !== 0 &&
      r.out.includes(`kapi ${verb} is not available in the browser`) &&
      !/unknown command/.test(r.out);
    ok(`\`kapi ${argv.join(" ")}\` explains the browser limitation`, honest, r.out.trim());
  }
}

// ── 3. The verbs the labs actually call, with their real argv ──────────────
//
// Each block replays a lab component's own invocation. A rename that breaks the
// component fails here rather than in a reader's browser.
console.log("command-surface-smoke: lab components' real invocations");
mem.vol.mkdirp("/project");

// ConversionExplorer (packages/kapi-lab/src/ConversionExplorer.tsx): queries the
// generative writers, then converts through the toolbox `kconv` proxy.
const MARKDOWN = `# Release notes

A paragraph with **bold** and a [link](https://example.com).

| Format | Kind |
| ------ | ---- |
| md     | text |
`;
mem.vol.writeFile("/project/article.md", enc.encode(MARKDOWN));

const formats = await run("formats", "list", "--json");
ok("ConversionExplorer: `formats list --json` exits 0", formats.code === 0, formats.out.trim());
let generative: string[] = [];
try {
  const parsed = JSON.parse(formats.out) as {
    formats?: { name: string; has_writer?: boolean; generative?: boolean; interchange?: boolean }[];
  };
  generative = (parsed.formats ?? [])
    .filter((f) => f.has_writer && f.generative && !f.interchange)
    .map((f) => f.name);
} catch (e) {
  ok("ConversionExplorer: formats JSON parses", false, String(e));
}
ok(
  "ConversionExplorer: the engine reports generative targets",
  generative.length > 0,
  `${generative.length} targets`,
);

// Convert to each of ConversionExplorer's own GENERATIVE_TARGETS — the pills it
// opens on and falls back to. Every one must serialize a prose document, since
// that is what the Conversion Lab's samples are. (The engine reports a wider
// generative set, some of it keyed-catalog or media-only; the lab surfaces those
// too, and a pill whose writer rejects a given input reports the error inline.)
const LAB_TARGETS = ["doclang", "markdown", "html", "asciidoc", "json", "yaml", "plaintext"];
for (const fmt of LAB_TARGETS) {
  ok(
    `ConversionExplorer: the engine still reports \`${fmt}\` as generative`,
    generative.includes(fmt),
    generative.join(" "),
  );
}
for (const fmt of LAB_TARGETS) {
  const outPath = `/project/converted-${fmt}`;
  const r = await run("kconv", "/project/article.md", "--to", fmt, "-o", outPath);
  let written: Uint8Array | null = null;
  try {
    written = mem.vol.readFile(outPath);
  } catch {
    written = null;
  }
  ok(
    `ConversionExplorer: \`kconv --to ${fmt}\` writes output`,
    r.code === 0 && written != null && written.length > 0,
    r.code === 0 ? "" : r.out.trim().split("\n").slice(0, 2).join(" | "),
  );
}

// The regression that started this: the old spelling must stay gone, so nobody
// reintroduces a bare `convert` alias and quietly re-splits the surface.
const retired = await run("convert", "/project/article.md", "--to", "markdown");
ok(
  "the retired bare `convert` spelling is not a command",
  retired.code !== 0 && /unknown command/.test(retired.out),
  retired.out.trim(),
);

// WorkspaceExplorer (packages/kapi-lab/src/WorkspaceExplorer.tsx).
const ws = await run("extract", "/project/article.md", "-o", "/project/w.kpz", "--target-lang", "qps");
ok("WorkspaceExplorer: `extract` to a workspace exits 0", ws.code === 0, ws.out.trim());
const info = await run("info", "/project/w.kpz", "--json");
ok("WorkspaceExplorer: `info --json` exits 0", info.code === 0, info.out.trim());

// TryNeokapi modal (web/src/components/TryNeokapi/ModalBody.tsx).
const pseudo = await run(
  "pseudo-translate",
  "/project/article.md",
  "-o",
  "/project/out-article.md",
  "--target-lang",
  "fr",
);
ok("TryNeokapi: `pseudo-translate` exits 0", pseudo.code === 0, pseudo.out.trim());

// Every lab on a page shares one engine and one /project, and a recipe run
// leaves a `.kapi/` state dir beside its recipe. The recipe-writing labs each
// keep their `kapi.yaml` in a directory of their own, so no state dir lands in
// /project without a recipe beside it, and a later command there that walks up
// for a project (the pseudo-translate widget, the playground terminal) still
// runs. FlowBuilderRunner writes /project/flow/kapi.yaml; ToolDropWidget writes
// /project/<instance id>/kapi.yaml.
const LAB_RECIPE =
  "version: v1\nname: Lab\ndefaults:\n  source_language: en\nflows:\n  lab:\n    steps:\n      - tool: pseudo-translate\n";
for (const [lab, dir] of [
  ["FlowBuilderRunner", "/project/flow"],
  ["ToolDropWidget", "/project/_r_0_"],
] as const) {
  mem.vol.mkdirp(dir);
  mem.vol.writeFile(`${dir}/kapi.yaml`, enc.encode(LAB_RECIPE));
  const r = await run(
    "run",
    "lab",
    "-p",
    `${dir}/kapi.yaml`,
    "-i",
    "/project/article.md",
    "-o",
    `${dir}-out-article.md`,
    "--target-lang",
    "fr",
  );
  ok(`${lab}: \`run lab -p ${dir}/kapi.yaml\` exits 0`, r.code === 0, r.out.trim());
}
const afterLabs = await run(
  "pseudo-translate",
  "/project/article.md",
  "-o",
  "/project/out-after-labs.md",
  "--target-lang",
  "qps",
);
ok(
  "a command in /project still runs after the recipe labs ran",
  afterLabs.code === 0,
  afterLabs.out.trim(),
);

// The playground's sample projects (packages/kapi-playground/src/samples.ts)
// seed their recipe into the terminal's working directory as kapi.yaml, and the
// staged funnel runs with no -p, so each step finds the project by discovery.
mem.vol.mkdirp("/sample");
mem.vol.writeFile(
  "/sample/kapi.yaml",
  enc.encode(
    'version: v1\nname: demo\ndefaults:\n  source_language: en\n  target_languages: [fr]\ncollections:\n  - path: messages.json\n    format: json\n    target: "out/{lang}/messages.json"\nflows:\n  translate:\n    steps:\n      - tool: recycle\n',
  ),
);
mem.vol.writeFile("/sample/messages.json", enc.encode('{"greeting": "Welcome to Acme"}\n'));
mem.process.chdir("/sample");
for (const argv of [["status"], ["extract"]]) {
  const r = await run(...argv);
  ok(`playground sample project: \`kapi ${argv.join(" ")}\` finds the project`, r.code === 0, r.out.trim());
}
mem.process.chdir("/project");

// ── 4. Verbs newly reachable in the browser actually work ──────────────────
console.log("command-surface-smoke: newly wired verbs run for real");
const inspect = await run("inspect", "/project/article.md");
ok(
  "`kapi inspect` emits anchored blocks",
  inspect.code === 0 && inspect.out.includes("content_hash"),
  inspect.out.trim().slice(0, 160),
);
const check = await run("check", "/project/article.md", "--no-fail");
ok("`kapi check` runs the default checkset", check.code === 0, check.out.trim().slice(0, 160));
const brand = await run("voice", "guide", "--pack", "technical-docs");
ok("`kapi voice guide --pack` works offline", brand.code === 0, brand.out.trim().slice(0, 160));

// Installing a profile writes to a SQLite voice store, which the browser holds
// in SQLite's WebAssembly build like every other store.
const brandStore = await run("voice", "pack", "technical-docs");
ok(
  "`kapi voice pack` installs into the voice store",
  brandStore.code === 0 && /created voice profile/.test(brandStore.out),
  brandStore.out.trim().slice(0, 240),
);
// A standalone store is consulted only once it is there, and only the driver
// can say so: the page's file system never sees a database.
const installed = await run("voice", "guide", "--profile", "technical-documentation");
ok(
  "`kapi voice guide --profile` reads the installed profile from the voice store",
  installed.code === 0 && installed.out.includes("Voice Guide: Technical Documentation"),
  installed.out.trim().slice(0, 240),
);

// A tool that needs terms reads the rules from the store --termstore names.
// The trace carries the finding: the source says "dashboard" and the target
// lacks the preferred French term.
mem.vol.mkdirp("/termstore");
mem.process.chdir("/termstore");
mem.vol.writeFile(
  "/termstore/terms.json",
  enc.encode(
    JSON.stringify({
      schemaVersion: "1.0",
      kind: "kapi-terms",
      concepts: [
        {
          id: "term:en:dashboard",
          terms: [
            { text: "dashboard", locale: "en", status: "approved" },
            { text: "tableau de bord", locale: "fr", status: "preferred" },
          ],
        },
      ],
    }),
  ),
);
mem.vol.writeFile(
  "/termstore/login.xlf",
  enc.encode(
    '<?xml version="1.0" encoding="UTF-8"?>\n<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">\n' +
      '<file original="messages" source-language="en" target-language="fr" datatype="plaintext"><body>\n' +
      '<trans-unit id="login_title"><source>Log in to your dashboard</source><target>Connectez-vous à votre panneau</target></trans-unit>\n' +
      "</body></file></xliff>\n",
  ),
);
const imported = await run("terms", "import", "terms.json");
ok("`kapi terms import` creates ./terms.db", imported.code === 0, imported.out.trim().slice(0, 160));
const termCheck = await run(
  "exec", "term-check", "login.xlf", "--source-lang", "en", "--target-lang", "fr",
  "--termstore", "terms.db", "--trace", "/termstore/trace.json",
);
const trace = termCheck.code === 0 ? dec.decode(mem.vol.readFile("/termstore/trace.json")) : "";
ok(
  "`kapi exec term-check --termstore` applies the store's rules",
  trace.includes('required translation \\"tableau de bord\\" missing'),
  termCheck.out.trim().slice(0, 240),
);

// A walkthrough's Reset starts its directory over (KapiRuntime.reset): the
// project there is forgotten with its context, and its databases go with its
// files, so the same project seeded again holds nothing.
const resetRecipe =
  "version: v1\nname: reset-demo\ndefaults:\n  source_language: en\n  target_languages: [fr]\n";
mem.vol.mkdirp("/reset-demo");
mem.process.chdir("/reset-demo");
mem.vol.writeFile("/reset-demo/kapi.yaml", enc.encode(resetRecipe));
mem.vol.writeFile("/reset-demo/terms.json", mem.vol.readFile("/termstore/terms.json"));
await run("terms", "import", "terms.json");
await run("terms", "import", "terms.json", "--termstore", "standalone.db");
const before = await run("terms", "stats");
ok("a project's terms import lands in its store", /Concepts:\s+1\b/.test(before.out), before.out.trim().slice(0, 160));
const resetFailure = await (globalThis as any).kapiReset("/reset-demo");
ok("kapiReset starts the directory over", resetFailure === null, String(resetFailure));
for (const name of mem.vol.readdir("/reset-demo")) mem.vol.remove(`/reset-demo/${name}`);
ok(
  "kapiReset leaves no database in the directory",
  (globalThis as any).__kapiSQL.list("/reset-demo").length === 0,
  JSON.stringify((globalThis as any).__kapiSQL.list("/reset-demo")),
);
mem.vol.writeFile("/reset-demo/kapi.yaml", enc.encode(resetRecipe));
const after = await run("terms", "stats");
ok("the project seeded again starts with no terms", /Concepts:\s+0\b/.test(after.out), after.out.trim().slice(0, 160));
mem.process.chdir("/project");

// The checkout MessageFormat fixtures in the lab samples: read, check and write
// each one with the inspector options the labs use.
console.log("command-surface-smoke: checkout fixtures");
for (const id of ["checkout-messageformat", "checkout-checks"]) {
  const sample = SAMPLES.find((entry) => entry.id === id)!;
  const path = `/project/${sample.filename}`;
  mem.vol.writeFile(path, enc.encode(sample.content));
  const inspected = await (globalThis as any).labInspect(path);
  ok(`${id}: real reader succeeds`, inspected.ok === true, inspected.error ?? "");
  const annotated = await (globalThis as any).labInspectAnnotated(path, JSON.stringify({ qa: true, term: false, brand: false, segment: false }));
  ok(`${id}: hygiene inspector succeeds`, annotated.ok === true, annotated.error ?? "");
  const tree = JSON.parse(inspected.json ?? "{}");
  const blocks = tree.root?.[0]?.children ?? [];
  ok(`${id}: name is a real placeholder`, blocks[0]?.source?.some((r: { ph?: { data?: string } }) => r.ph?.data === "{name}"));
  for (const branch of ["count.=0", "count.one", "count.other"]) {
    ok(`${id}: preserves plural branch ${branch}`, blocks.some((b: { properties?: { path?: string } }) => b.properties?.path === branch));
  }
  const checkedTree = JSON.parse(annotated.json ?? "{}");
  const checkJSON = JSON.stringify(checkedTree.root);
  for (const category of ["double-spaces", "doubled-word"]) {
    ok(`${id}: ${category} finding matches fixture`, checkJSON.includes(`"category":"${category}"`) === (id === "checkout-checks"));
  }
  const written = await run("pseudo-translate", path, "-o", `/project/pseudo-${sample.filename}`);
  ok(`${id}: pseudo-translation writes output`, written.code === 0, written.out.trim().slice(0, 160));
  if (written.code === 0) {
    const output = dec.decode(mem.vol.readFile(`/project/pseudo-${sample.filename}`));
    ok(`${id}: writer changes text`, output !== sample.content);
    ok(`${id}: writer retains placeholder and plural selectors`, output.includes("{name}") && output.includes("{count, plural,") && output.includes("one {") && output.includes("other {"));
    ok(`${id}: writer applies edits inside plural branches`, !output.includes("Your cart is empty") && !output.includes("item is ready for checkout") && !output.includes("items are ready for checkout"));
  }
}

// Execute the exact strings shown in the sample picker and terminal help.
// No Translator bridge is installed in Node, so the browser provider takes
// its documented deterministic demo fallback without model downloads or API calls.
console.log("command-surface-smoke: public CLI examples");
for (const sample of LOOSE_SAMPLES) {
  const directory = `/cli-example-${sample.id}`;
  mem.vol.mkdirp(directory);
  mem.process.chdir(directory);
  mem.vol.writeFile(`${directory}/${sample.file.path}`, enc.encode(sample.file.content));
  const result = await run(...parseCommand(sample.suggested));
  ok(`sample ${sample.id}: ${sample.suggested}`, result.code === 0, result.out.trim().slice(0, 180));
}
mem.vol.mkdirp("/cli-help");
mem.process.chdir("/cli-help");
mem.vol.writeFile("/cli-help/messages.json", enc.encode(getFixture("messages.json")!.content));
for (const command of CLI_EXAMPLES) {
  const result = await run(...parseCommand(command));
  ok(`terminal help: ${command}`, result.code === 0, result.out.trim().slice(0, 180));
  if (command.includes("--jq")) ok("stats jq selects a numeric word count", /^\d+\s*$/.test(result.out.trim()), result.out.trim());
}

// Walkthrough embeds carry runnable command sequences and their complete seeds.
// Import each generated config directly so the guard exercises what the page runs.
const embedDirectory = join(REPO_ROOT, "web/src/components/KapiPlayground/embeds");
for (const filename of readdirSync(embedDirectory).filter((name) => name.endsWith(".embed.ts"))) {
  const { default: config } = await import(pathToFileURL(join(embedDirectory, filename)).href);
  const directory = `/embed-${config.id}`;
  mem.vol.mkdirp(directory);
  mem.process.chdir(directory);
  for (const fixture of config.seed ?? []) {
    const file = getFixture(fixture);
    if (!file) throw new Error(`Unknown embed fixture ${fixture}`);
    mem.vol.mkdirp(dirname(`${directory}/${file.name}`));
    mem.vol.writeFile(`${directory}/${file.name}`, enc.encode(file.content));
  }
  for (const file of config.files ?? []) {
    mem.vol.mkdirp(dirname(`${directory}/${file.path}`));
    mem.vol.writeFile(`${directory}/${file.path}`, enc.encode(file.content));
  }
  for (const step of config.steps) {
    const result = await run(...parseCommand(step.command));
    ok(`${config.id}: ${step.command}`, result.code === 0, result.out.trim().slice(0, 180));
  }
}

console.log(failures === 0 ? "\ncommand-surface-smoke: all checks passed" : `\ncommand-surface-smoke: ${failures} failure(s)`);
process.exit(failures === 0 ? 0 : 1);
