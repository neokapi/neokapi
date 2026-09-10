// Reproduce deterministic audience coverage with an isolated, explicit project.
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, "../..");
const args = process.argv.slice(2);
const option = (name, fallback) => {
  const index = args.indexOf(name);
  if (index < 0) return fallback;
  if (!args[index + 1]) throw new Error(`${name} requires a value`);
  return args[index + 1];
};
const binary = resolve(option("--binary", join(repo, "bin/kapi")));
const output = resolve(option("--output", join(tmpdir(), "audience-results.json")));
const sandbox = mkdtempSync(join(tmpdir(), "kapi-audience-"));
const hash = (bytes) => createHash("sha256").update(bytes).digest("hex");
const git = (...gitArgs) => execFileSync("git", gitArgs, { cwd: repo, encoding: "utf8" }).trim();
const env = {
  ...process.env,
  KAPI_NO_PROJECT: "1",
  KAPI_PROJECT: "",
  KAPI_CONFIG_DIR: join(sandbox, "isolated/config"),
  XDG_DATA_HOME: join(sandbox, "isolated/data"),
  XDG_CACHE_HOME: join(sandbox, "isolated/cache"),
  KAPI_PLUGINS_DIR_ONLY: "1",
  KAPI_PLUGINS_DIR: join(sandbox, "isolated/plugins"),
  KAPI_TELEMETRY: "0",
};
const normalize = (value) => JSON.parse(JSON.stringify(value).replaceAll(sandbox, "<fixture>"));
const results = [];
let version;
let lastAttempt;
let activeCase;
let failure;
const optional = (read) => {
  try { return read(); } catch { return undefined; }
};
const invoke = (command) => {
  const run = spawnSync(binary, command, {
    cwd: sandbox, env, encoding: "utf8", timeout: 30_000, maxBuffer: 8 * 1024 * 1024,
  });
  lastAttempt = {
    command, exitCode: run.status, signal: run.signal,
    stdout: run.stdout ?? "", stderr: run.stderr ?? "",
    error: run.error ? { message: run.error.message, code: run.error.code } : undefined,
  };
  if (run.error) throw run.error;
  return lastAttempt;
};

try {
  for (const path of [".kapi", "kapi.yaml", "site", "service-facts.md"]) {
    cpSync(join(here, path), join(sandbox, path), { recursive: true });
  }
  version = invoke(["--version"]);
  if (version.exitCode !== 0) throw new Error(version.stderr);
  const cases = [
    { id: "clean", text: null, expected: 0 },
    { id: "assurance", text: "A video appointment is risk-free.", expected: 1 },
    { id: "repair", text: "A video appointment is not recorded. Screen sharing requires your consent, and you can stop sharing at any time. The adviser cannot control your device or open its files.", expected: 0 },
    { id: "semantic-contradiction", text: "A video appointment is recorded. Screen sharing starts automatically and you cannot stop it. Harbor Help can control your device and open all your files.", expected: 0 },
    { id: "mechanical", text: "The the appointment is ready.", expected: 0 },
  ];
  for (const audience of ["child", "teen", "adult", "older-adult"]) {
    const path = `site/locales/en/${audience}.json`;
    const original = JSON.parse(readFileSync(join(here, path), "utf8"));
    for (const fixture of cases) {
      const input = fixture.text ? { ...original, fact_body: fixture.text } : original;
      const bytes = JSON.stringify(input, null, 2) + "\n";
      activeCase = { id: `${audience}/${fixture.id}`, file: path, input, inputSha256: hash(bytes) };
      writeFileSync(join(sandbox, path), bytes);
      const context = invoke(["context", path, "--json", "-p", "kapi.yaml"]);
      if (context.exitCode !== 0) throw new Error(context.stderr);
      const resolvedContext = JSON.parse(context.stdout);
      const run = invoke(["check", path, "--json", "-p", "kapi.yaml"]);
      const report = JSON.parse(run.stdout);
      const actual = (report.findings ?? []).filter((finding) =>
        finding.metadata?.constraint_id === "harbor-help/no-unsupported-assurance").length;
      const guidanceVisible = resolvedContext.constraints?.some((entry) =>
        entry.constraint.kind === "guidance" && entry.status === "applicable");
      const guidanceUnsupported = report.execution?.analyzers?.some((entry) =>
        entry.id === "voice.guidance" && entry.status === "unsupported" && entry.required === false);
      const expectedExit = fixture.expected > 0 ? 3 : 0;
      const passed = actual === fixture.expected && run.exitCode === expectedExit &&
        guidanceVisible && guidanceUnsupported;
      results.push(normalize({
        id: `${audience}/${fixture.id}`, audience, file: path, input, inputSha256: hash(bytes),
        expectedConstraintFindings: fixture.expected, actualConstraintFindings: actual,
        expectedExit, passed, resolvedContext, contextRun: context, run, report,
        semanticVerification: guidanceUnsupported ? "unsupported" : "unreported",
      }));
      process.stdout.write(`${passed ? "PASS" : "FAIL"} ${audience}/${fixture.id}: ${actual} constraint findings, exit ${run.exitCode}, guidance ${guidanceUnsupported ? "unsupported" : "unreported"}\n`);
    }
  }
  if (results.some((result) => !result.passed)) process.exitCode = 1;
} catch (error) {
  failure = normalize({ message: error.message, case: activeCase, attempt: lastAttempt });
  process.stderr.write(`Audience fixture failed: ${error.message}\n`);
  process.exitCode = 1;
} finally {
  const artifact = {
    schema: "neokapi-audience-coverage/v1", recordedAt: new Date().toISOString(),
    sourceCommit: optional(() => git("rev-parse", "HEAD")),
    sourceDirty: optional(() => git("status", "--porcelain", "--untracked-files=no") !== ""),
    sourceDiffSha256: optional(() => hash(git("diff", "HEAD"))),
    binarySha256: optional(() => hash(readFileSync(binary))), binaryVersion: version?.stdout.trim(),
    profileSha256: optional(() => hash(readFileSync(join(here, ".kapi/voice.yaml")))),
    recipeSha256: optional(() => hash(readFileSync(join(here, "kapi.yaml")))),
    scope: "Offline deterministic checks; no model, remote binding or authenticated review.",
    limitations: [
      "Literal assurance matching does not establish factual truth.",
      "No latency benchmark or comparative productivity experiment was run.",
      "The caller must build the binary; version and hashes identify the supplied artifact.",
    ],
    results, failure,
  };
  try {
    mkdirSync(dirname(output), { recursive: true });
    writeFileSync(output, JSON.stringify(artifact, null, 2) + "\n");
    process.stdout.write(`Saved ${output}\n`);
  } finally {
    rmSync(sandbox, { recursive: true, force: true });
  }
}
