#!/usr/bin/env node
// EngineService smoke-test client (Node).
//
// Starts `kapi engine serve`, then drives the full contract against it:
//
//   1. Extract  — stream messages.json in chunks, collect the Part stream
//   2. Merge    — identity round-trip: merged bytes must equal the source
//   3. Process  — pseudo-translate the parts (target locale qps)
//   4. Merge    — write the pseudo-translated document
//   5. Parity   — run the same pipeline through the kapi CLI and assert the
//                 engine output is byte-identical
//
// The .proto files are loaded dynamically with @grpc/proto-loader — no
// codegen step; the checked-in .proto files are the contract.
//
// Environment:
//   KAPI_BIN   path to the kapi binary (default: "kapi" on PATH)
//   REPO_ROOT  repo root holding core/proto/ (default: ../../ from this file)

import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { createInterface } from "node:readline";
import { fileURLToPath } from "node:url";

import grpc from "@grpc/grpc-js";
import protoLoader from "@grpc/proto-loader";

const HERE = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = process.env.REPO_ROOT ?? resolve(HERE, "..", "..");
const KAPI_BIN = process.env.KAPI_BIN ?? "kapi";
const FIXTURE = join(HERE, "messages.json");

function loadEngineService() {
  const def = protoLoader.loadSync("core/proto/engine/v1/engine.proto", {
    includeDirs: [REPO_ROOT],
    keepCase: false,
    longs: Number,
    defaults: true,
    oneofs: true,
  });
  return grpc.loadPackageDefinition(def).neokapi.engine.v1.EngineService;
}

function startServer() {
  const proc = spawn(KAPI_BIN, ["engine", "serve"], {
    stdio: ["ignore", "pipe", "ignore"],
  });
  return new Promise((resolvePromise, reject) => {
    proc.once("error", reject);
    proc.once("exit", (code) =>
      reject(new Error(`kapi engine serve exited early (code ${code})`)),
    );
    createInterface({ input: proc.stdout }).once("line", (line) => {
      proc.removeAllListeners("exit");
      resolvePromise({ proc, socket: JSON.parse(line).socket });
    });
  });
}

// Drives one client-streaming→server-streaming call: sends the messages,
// half-closes, and collects the responses.
function roundTrip(stream, messages) {
  return new Promise((resolvePromise, reject) => {
    const responses = [];
    stream.on("data", (msg) => responses.push(msg));
    stream.on("error", reject);
    stream.on("end", () => resolvePromise(responses));
    for (const msg of messages) stream.write(msg);
    stream.end();
  });
}

function* chunked(buf, size = 64 * 1024) {
  for (let i = 0; i < buf.length; i += size) {
    yield buf.subarray(i, i + size);
  }
}

function collectParts(responses) {
  return responses.flatMap((r) => (r.parts ? r.parts.parts : []));
}

function collectBytes(responses) {
  return Buffer.concat(
    responses.filter((r) => r.chunk).map((r) => r.chunk.data),
  );
}

function assert(cond, msg) {
  if (!cond) {
    console.error(`FAIL: ${msg}`);
    process.exit(1);
  }
}

const EngineService = loadEngineService();
const { proc, socket } = await startServer();
const workdir = mkdtempSync(join(tmpdir(), "kapi-engine-node-"));

try {
  const client = new EngineService(
    `unix://${socket}`,
    grpc.credentials.createInsecure(),
  );
  await new Promise((resolvePromise, reject) =>
    client.waitForReady(Date.now() + 15000, (err) =>
      err ? reject(err) : resolvePromise(),
    ),
  );

  const source = readFileSync(FIXTURE);

  // 1. Extract.
  const extractResponses = await roundTrip(client.extract(), [
    { header: { name: "messages.json", sourceLocale: "en" } },
    ...[...chunked(source)].map((data) => ({ chunk: { data } })),
  ]);
  const parts = collectParts(extractResponses);
  assert(parts.length > 0, "extract returned no parts");

  const mergeMessages = (mergeParts, targetLocale = "") => [
    {
      header: {
        name: "messages.json",
        original: { inline: source },
        targetLocale,
      },
    },
    { parts: { parts: mergeParts } },
  ];

  // 2. Identity merge: byte-exact round trip.
  const identity = collectBytes(
    await roundTrip(client.merge(), mergeMessages(parts)),
  );
  assert(
    identity.equals(source),
    `identity extract→merge is not byte-exact:\n--- source ---\n${source}\n--- merged ---\n${identity}`,
  );
  console.log("ok: identity round-trip is byte-exact");

  // 3. Process: pseudo-translate to qps.
  const processResponses = await roundTrip(client.process(), [
    {
      header: {
        tools: [{ tool: "pseudo-translate" }],
        sourceLocale: "en",
        targetLocale: "qps",
      },
    },
    { parts: { parts } },
  ]);
  const processed = collectParts(processResponses);
  assert(processed.length > 0, "process returned no parts");

  // 4. Merge the pseudo-translated parts.
  const engineOut = collectBytes(
    await roundTrip(client.merge(), mergeMessages(processed, "qps")),
  );
  assert(!engineOut.equals(source), "pseudo-translate produced no change");
  console.log("ok: pseudo-translate + merge produced a localized document");

  // 5. CLI parity: byte-identical to `kapi pseudo-translate`.
  const fixtureCopy = join(workdir, "messages.json");
  writeFileSync(fixtureCopy, source);
  const cliOut = join(workdir, "cli-out.json");
  execFileSync(
    KAPI_BIN,
    ["pseudo-translate", fixtureCopy, "-o", cliOut, "--target-lang", "qps"],
    { stdio: "ignore" },
  );
  const cliBytes = readFileSync(cliOut);
  assert(
    engineOut.equals(cliBytes),
    `engine output differs from CLI output:\n--- engine ---\n${engineOut}\n--- cli ---\n${cliBytes}`,
  );
  console.log("ok: engine output is byte-identical to the CLI output");
  console.log("PASS engine-client-node");
} finally {
  proc.kill("SIGTERM");
  rmSync(workdir, { recursive: true, force: true });
}
