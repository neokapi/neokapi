import fs from "node:fs";
import path from "node:path";
import { chromium, type Browser } from "playwright";
import type { ArtifactSpec, CapturedArtifact, DemoManifest } from "../types.ts";
import { captureDir, demoFixturesDir, ensureDir, kapiIsolationEnv, publicDemoDir, REPO_ROOT } from "../lib/paths.ts";
import { spawn } from "node:child_process";
import { sh, sleep } from "../lib/exec.ts";
import { renderReport, renderMarkdownDoc, renderCode, renderCodeDiff, renderDocxHtml } from "./report.ts";

const DEFAULT_W = 1280;
const DEFAULT_H = 800;

/** Parse simple CSV (header row → keys) into rows of objects, for the glossary report. */
function parseCsv(text: string): Array<Record<string, string>> {
  const lines = text.split("\n").map((l) => l.trim()).filter(Boolean);
  if (lines.length < 2) return [];
  const headers = lines[0].split(",").map((h) => h.trim());
  return lines.slice(1).map((line) => {
    const cells = line.split(",").map((c) => c.trim());
    return Object.fromEntries(headers.map((h, i) => [h, cells[i] ?? ""]));
  });
}

/** Pull the first JSON object/array out of command stdout (tolerates leading log noise). */
function extractJson(stdout: string): any {
  const s = stdout.indexOf("{");
  const a = stdout.indexOf("[");
  const start = s < 0 ? a : a < 0 ? s : Math.min(s, a);
  if (start < 0) throw new Error("no JSON in output");
  const open = stdout[start];
  const close = open === "{" ? "}" : "]";
  const end = stdout.lastIndexOf(close);
  return JSON.parse(stdout.slice(start, end + 1));
}

function reportEnv() {
  return { ...process.env, PATH: `${path.join(REPO_ROOT, "bin")}:${process.env.PATH}`, ...kapiIsolationEnv() };
}

/** Parse a Java .properties file into a key→value object (for the catalog report). */
function parseProperties(text: string): Record<string, string> {
  // Java .properties escape non-ASCII as \uXXXX — decode for display.
  const unescape = (s: string) => s.replace(/\\u([0-9a-fA-F]{4})/g, (_, h) => String.fromCharCode(parseInt(h, 16)));
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t || t.startsWith("#") || t.startsWith("!")) continue;
    const eq = t.search(/[=:]/);
    if (eq < 0) continue;
    out[t.slice(0, eq).trim()] = unescape(t.slice(eq + 1).trim());
  }
  return out;
}

/** The final sandbox state captured during the run is the source of truth for artifacts. */
function snapshotDir(id: string): string {
  return path.join(captureDir(id), "sandbox");
}

async function shot(browser: Browser, url: string, out: string, w: number, h: number, settleMs = 500): Promise<void> {
  const page = await browser.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: 2 });
  try {
    await page.goto(url, { waitUntil: "networkidle", timeout: 30_000 }).catch(() => page.goto(url, { timeout: 30_000 }));
    // settleMs lets client-side work finish — e.g. a kapi-react runtime locale
    // swap (fetch /translations/<lang>.json, then re-render) before the shot.
    await page.waitForTimeout(settleMs);
    await page.screenshot({ path: out });
  } finally {
    await page.close();
  }
}

/** Poll a URL until it responds (any HTTP status), for servers that take time to boot (e.g. `next build && next start`). */
async function waitForServer(url: string, timeoutMs = 180_000): Promise<boolean> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      await fetch(url);
      return true;
    } catch {
      await sleep(1500);
    }
  }
  return false;
}

async function captureOne(
  browser: Browser,
  m: DemoManifest,
  spec: ArtifactSpec,
): Promise<CapturedArtifact | null> {
  const snap = snapshotDir(m.id);
  // Read from the pristine fixture (for a "before" card) or the post-run snapshot.
  const baseDir = spec.from === "fixture" ? demoFixturesDir(m.id) : snap;
  const pub = publicDemoDir(m.id);
  const artDir = ensureDir(path.join(pub, "artifacts"));
  const w = spec.width ?? DEFAULT_W;
  const h = spec.height ?? DEFAULT_H;
  const outPng = path.join(artDir, `${spec.id}.png`);

  try {
    if (spec.source === "image") {
      const src = path.join(baseDir, spec.path ?? "");
      if (!fs.existsSync(src)) {
        console.warn(`    skip artifact ${spec.id}: image ${spec.path} not found`);
        return null;
      }
      fs.copyFileSync(src, outPng);
    } else if (spec.source === "html") {
      const src = path.join(baseDir, spec.path ?? "");
      if (!fs.existsSync(src)) {
        console.warn(`    skip artifact ${spec.id}: html ${spec.path} not found`);
        return null;
      }
      await shot(browser, "file://" + src, outPng, w, h);
    } else if (spec.source === "report") {
      const src = path.join(baseDir, spec.path ?? "");
      if (!fs.existsSync(src)) {
        console.warn(`    skip artifact ${spec.id}: report source ${spec.path} not found`);
        return null;
      }
      let html: string;
      if (spec.report === "markdown") {
        html = renderMarkdownDoc(fs.readFileSync(src, "utf8"), spec.reportTitle ?? "Document");
      } else if (spec.report === "code") {
        html = renderCode(fs.readFileSync(src, "utf8"), spec.reportTitle ?? path.basename(src), spec.reportSub ?? "");
      } else {
        const text = fs.readFileSync(src, "utf8");
        let json: any;
        if (src.endsWith(".properties")) json = parseProperties(text);
        else if (src.endsWith(".csv")) json = parseCsv(text);
        else {
          try {
            json = JSON.parse(text);
          } catch {
            console.warn(`    skip artifact ${spec.id}: ${spec.path} is not valid JSON`);
            return null;
          }
        }
        html = renderReport(spec.report ?? "json", json, { title: spec.reportTitle, sub: spec.reportSub });
      }
      const tmpHtml = path.join(artDir, `${spec.id}.html`);
      fs.writeFileSync(tmpHtml, html);
      await shot(browser, "file://" + tmpHtml, outPng, w, h);
    } else if (spec.source === "codediff") {
      // Same file from the pristine fixture (before) and the post-run snapshot (after).
      const beforeSrc = path.join(demoFixturesDir(m.id), spec.path ?? "");
      const afterSrc = path.join(snap, spec.path ?? "");
      if (!fs.existsSync(beforeSrc) || !fs.existsSync(afterSrc)) {
        console.warn(`    skip artifact ${spec.id}: codediff source ${spec.path} missing (before/after)`);
        return null;
      }
      const html = renderCodeDiff(
        fs.readFileSync(beforeSrc, "utf8"),
        fs.readFileSync(afterSrc, "utf8"),
        spec.reportTitle ?? path.basename(spec.path ?? ""),
        spec.reportSub ?? "",
        "Before — plain JSX",
        "After — kapi-react",
      );
      const tmpHtml = path.join(artDir, `${spec.id}.html`);
      fs.writeFileSync(tmpHtml, html);
      await shot(browser, "file://" + tmpHtml, outPng, w, h);
    } else if (spec.source === "command") {
      // Run a kapi command in the snapshot and render its REAL stdout (deterministic).
      const r = await sh(spec.command!, { cwd: baseDir, env: reportEnv(), timeoutMs: 120_000 });
      let json: any;
      try {
        json = extractJson(r.stdout);
      } catch {
        console.warn(`    skip artifact ${spec.id}: command produced no JSON (exit ${r.code})`);
        return null;
      }
      const html = renderReport(spec.report ?? "json", json, { title: spec.reportTitle, sub: spec.reportSub });
      const tmpHtml = path.join(artDir, `${spec.id}.html`);
      fs.writeFileSync(tmpHtml, html);
      await shot(browser, "file://" + tmpHtml, outPng, w, h);
    } else if (spec.source === "url") {
      // Best-effort: start a server in the snapshot, screenshot, tear down. Spawn
      // detached (own process group) so teardown can kill the WHOLE tree: `pnpm run
      // dev` forks next-server + render workers, and killing only the port listener
      // leaves them alive — back-to-back url artifacts then stack multiple dev
      // servers and exhaust memory (OOM). process.kill(-pid) signals the group.
      const port = spec.port ?? 4599;
      const serveCmd = spec.serve ?? `python3 -m http.server ${port}`;
      const server = spawn("/bin/sh", ["-c", serveCmd], {
        cwd: snap,
        env: { ...process.env, PATH: `${path.join(REPO_ROOT, "bin")}:${process.env.PATH}` },
        detached: true,
        stdio: "ignore",
      });
      server.on("error", () => {});
      const ready = await waitForServer(`http://localhost:${port}/`, spec.serveTimeoutMs ?? 180_000);
      if (!ready) console.warn(`    artifact ${spec.id}: server on :${port} not ready in time, screenshotting anyway`);
      try {
        await shot(browser, `http://localhost:${port}${spec.path ?? "/"}`, outPng, w, h, spec.settleMs ?? 500);
      } finally {
        // Kill the whole process group (npm → next → workers), then sweep the port.
        if (server.pid) {
          try {
            process.kill(-server.pid, "SIGKILL");
          } catch {
            /* group already gone */
          }
        }
        await sh(`lsof -ti tcp:${port} | xargs kill -9 2>/dev/null || true`);
      }
      if (!fs.existsSync(outPng)) {
        console.warn(`    skip artifact ${spec.id}: url capture produced nothing`);
        return null;
      }
    } else if (spec.source === "docx") {
      // Render a .docx via pandoc → HTML fragment → styled document card; the
      // structure (headings, lists, bold) carries through, so before/after shots
      // show the real document, translated.
      const src = path.join(baseDir, spec.path ?? "");
      if (!fs.existsSync(src)) {
        console.warn(`    skip artifact ${spec.id}: docx ${spec.path} not found`);
        return null;
      }
      const r = await sh(`pandoc ${JSON.stringify(src)} -t html`, { cwd: baseDir, timeoutMs: 60_000 });
      if (r.code !== 0 || !r.stdout.trim()) {
        console.warn(`    skip artifact ${spec.id}: pandoc failed on ${spec.path} (exit ${r.code})`);
        return null;
      }
      const html = renderDocxHtml(r.stdout, spec.reportTitle ?? path.basename(src), spec.reportSub ?? "");
      const tmpHtml = path.join(artDir, `${spec.id}.html`);
      fs.writeFileSync(tmpHtml, html);
      await shot(browser, "file://" + tmpHtml, outPng, w, h);
    }

    const dims = spec.width && spec.height ? { width: spec.width, height: spec.height } : { width: w, height: h };
    return { id: spec.id, caption: spec.caption, image: `artifacts/${spec.id}.png`, width: dims.width, height: dims.height };
  } catch (e) {
    console.warn(`    skip artifact ${spec.id}: ${(e as Error).message.slice(0, 160)}`);
    return null;
  }
}

export interface ArtifactsOptions {
  force?: boolean;
}

/** Capture all declared artifacts for a demo → public/<id>/artifacts/*.png + artifacts.json. */
export async function captureArtifacts(m: DemoManifest, opts: ArtifactsOptions = {}): Promise<CapturedArtifact[]> {
  const pub = publicDemoDir(m.id);
  const artifactsJson = path.join(pub, "artifacts.json");
  if (!m.artifacts.length) {
    fs.writeFileSync(artifactsJson, "[]");
    return [];
  }
  if (!opts.force && fs.existsSync(artifactsJson)) {
    console.log(`  · artifacts exist for ${m.id} (use --force to re-run)`);
    return JSON.parse(fs.readFileSync(artifactsJson, "utf8"));
  }
  if (!fs.existsSync(snapshotDir(m.id))) {
    console.warn(`  ! no sandbox snapshot for ${m.id} — run capture first`);
    fs.writeFileSync(artifactsJson, "[]");
    return [];
  }

  console.log(`  · capturing ${m.artifacts.length} artifact(s) for ${m.id}`);
  const browser = await chromium.launch();
  const captured: CapturedArtifact[] = [];
  try {
    for (const spec of m.artifacts) {
      const c = await captureOne(browser, m, spec);
      if (c) captured.push(c);
    }
  } finally {
    await browser.close();
  }
  fs.writeFileSync(artifactsJson, JSON.stringify(captured, null, 2));
  console.log(`  ✓ artifacts ${m.id}: ${captured.length}/${m.artifacts.length} captured`);
  return captured;
}
