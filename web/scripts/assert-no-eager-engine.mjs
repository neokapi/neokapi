#!/usr/bin/env node
// assert-no-eager-engine.mjs — proves the site's no-run-on-load guarantee.
//
// Serves the production build (web/build) and, with Playwright request
// interception, asserts that loading each audited page issues ZERO requests
// for engine/model artifacts (\.wasm / \.onnx, their .gz variants, the
// same-origin /wasm/ + /models/ asset paths, and the known model CDN hosts) —
// then clicks that page's activation gate and asserts the requests DO fire.
//
// Playwright is resolved from harness/node_modules (the repo's only Playwright
// install — same pattern the harness itself uses); override with
// HARNESS_NODE_MODULES if your harness tree lives elsewhere. Uses channel
// "chrome" (the installed Google Chrome), so no browser download is needed.
//
// Usage: node web/scripts/assert-no-eager-engine.mjs
// Exits non-zero (with a per-page report) on any violation.

import { createRequire } from "node:module";
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..", "..");
const buildDir = path.join(repoRoot, "web", "build");

const harnessModules =
  process.env.HARNESS_NODE_MODULES ?? path.join(repoRoot, "harness", "node_modules");
const requireFromHarness = createRequire(path.join(harnessModules, "x.js"));
const { chromium } = requireFromHarness("playwright");

// A request is an engine/model artifact when it matches any of these. This is
// deliberately broader than "what exists today": any .wasm/.onnx anywhere, the
// site's own artifact paths, and the CDN hosts the bridges pull from
// (onnxruntime-web's runtime from jsdelivr, tokenizer/model weights from
// Hugging Face, the R2 docs CDN's wasm/model prefixes).
const ARTIFACT_PATTERNS = [
  /\.wasm(\.gz)?(\?|$)/i,
  /\.onnx(\.gz)?(\?|$)/i,
  /\/wasm\//i,
  /\/models\//i,
  /cdn\.jsdelivr\.net\/npm\/onnxruntime-web/i,
  /huggingface\.co/i,
];
const isArtifact = (url) => ARTIFACT_PATTERNS.some((re) => re.test(url));

// ── Tiny static server for the Docusaurus build output ──────────────────────
const MIME = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".css": "text/css",
  ".json": "application/json",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".ico": "image/x-icon",
  ".woff2": "font/woff2",
};
function serve(dir) {
  return createServer(async (req, res) => {
    try {
      const url = new URL(req.url ?? "/", "http://localhost");
      let p = path.join(dir, decodeURIComponent(url.pathname));
      if (existsSync(p) && !path.extname(p)) p = path.join(p, "index.html");
      if (!existsSync(p)) p = `${path.join(dir, decodeURIComponent(url.pathname))}.html`;
      if (!existsSync(p)) {
        res.writeHead(404).end("not found");
        return;
      }
      res.writeHead(200, { "content-type": MIME[path.extname(p)] ?? "application/octet-stream" });
      res.end(await readFile(p));
    } catch {
      res.writeHead(500).end();
    }
  });
}

// ── The audited pages: URL + how to activate that page's gate ───────────────
const PAGES = [
  {
    url: "/",
    label: "landing page",
    gate: {
      description: 'hero "Open the interactive Try Neokapi showcase" (modal boots the engine)',
      click: (page) => page.click('[aria-label="Open the interactive Try Neokapi showcase"]'),
    },
  },
  {
    url: "/kapi/get-started/quickstart",
    label: "docs page with RunnableSnippet embeds",
    gate: {
      description: "first RunnableSnippet ▸ Run (opens the shared modal, bootOnMount)",
      click: (page) => page.click(".kapi-pg-run-btn"),
    },
  },
  {
    url: "/labs",
    label: "labs overview",
    gate: null, // static index — links only, nothing to activate
  },
  {
    url: "/reference/formats/androidxml",
    label: "format reference page with a curated BlockPreview",
    gate: {
      description: 'compact RunGate "Read with kapi" (boots the shared engine)',
      click: (page) => page.click('button:has-text("Read with kapi")'),
    },
  },
  {
    url: "/lab/vision",
    label: "Vision Lab",
    gate: {
      description: 'RunGate "Run in your browser" (fetches the PP-OCRv5 ONNX models)',
      click: (page) => page.click('button:has-text("Run in your browser")'),
    },
  },
];

async function auditPage(browser, origin, spec) {
  const context = await browser.newContext();
  const page = await context.newPage();
  const requests = [];
  page.on("request", (r) => requests.push(r.url()));

  await page.goto(origin + spec.url, { waitUntil: "networkidle", timeout: 60_000 });
  // Grace period: catch anything a lazy chunk or idle callback might kick off.
  await page.waitForTimeout(3_000);

  const eager = requests.filter(isArtifact);
  const result = {
    url: spec.url,
    label: spec.label,
    total: requests.length,
    eager,
    afterClick: null,
    gate: spec.gate?.description ?? null,
    ok: eager.length === 0,
  };

  if (spec.gate && eager.length === 0) {
    const before = requests.length;
    await spec.gate.click(page);
    // The engine/model fetch fires immediately on activation; poll up to 30 s.
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      const fired = requests.slice(before).filter(isArtifact);
      if (fired.length > 0) {
        result.afterClick = fired;
        break;
      }
      await page.waitForTimeout(250);
    }
    if (!result.afterClick) {
      result.ok = false;
      result.afterClick = [];
    }
  }

  await context.close();
  return result;
}

const server = serve(buildDir);
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const origin = `http://127.0.0.1:${server.address().port}`;

const browser = await chromium.launch({ channel: "chrome", headless: true });
let failed = false;
console.log(`# no-eager-engine audit — ${new Date().toISOString()}`);
console.log(`# serving ${path.relative(repoRoot, buildDir)} at ${origin}\n`);
for (const spec of PAGES) {
  const r = await auditPage(browser, origin, spec);
  console.log(`## ${r.url}  (${r.label})`);
  console.log(
    `   requests on load: ${r.total}; engine/model artifacts BEFORE interaction: ${r.eager.length}`,
  );
  if (r.eager.length > 0) {
    failed = true;
    for (const u of r.eager) console.log(`   VIOLATION (eager): ${u}`);
  }
  if (r.gate) {
    if (r.afterClick && r.afterClick.length > 0) {
      console.log(`   gate: ${r.gate}`);
      console.log(`   after click: ${r.afterClick.length} artifact request(s) fired, e.g.`);
      for (const u of r.afterClick.slice(0, 4)) console.log(`     → ${u}`);
    } else if (r.eager.length === 0) {
      failed = true;
      console.log(`   VIOLATION: gate click (${r.gate}) fired NO artifact requests`);
    }
  } else {
    console.log("   gate: none on this page (static index)");
  }
  console.log(`   ${r.ok ? "PASS" : "FAIL"}\n`);
}
await browser.close();
server.close();
console.log(
  failed ? "RESULT: FAIL" : "RESULT: PASS — zero eager engine/model fetches; gates fire on click",
);
process.exit(failed ? 1 : 0);
