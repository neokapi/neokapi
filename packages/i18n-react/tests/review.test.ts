/**
 * In-context review Tier 1: transform stamping, the KBF-backed
 * review store, and the HTTP handler round-trip.
 */

import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { parseSync } from "@swc/core";
import { afterEach, describe, expect, it, vi } from "vitest";

import { extractDocument } from "../src/extract/walker.ts";
import { transform } from "../src/plugin/transform.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { ReviewStore, handleReviewRequest } from "../src/review/store.ts";

const tmp: string[] = [];
afterEach(() => {
  for (const d of tmp.splice(0)) rmSync(d, { recursive: true, force: true });
  vi.restoreAllMocks();
});

function scratch(): string {
  const dir = mkdtempSync(join(tmpdir(), "kapi-review-"));
  tmp.push(dir);
  return dir;
}

// ─── Transform stamping ──────────────────────────────────────

describe("review-mode stamping", () => {
  const rt = (code: string) =>
    transform(code, "src/Page.tsx", { mode: "runtime", review: true })?.code ?? null;

  it("stamps data-kapi-id and data-kapi-loc on extracted elements", () => {
    const out = rt("<h1>Welcome back</h1>");
    const hash = hashKey("Welcome back", "h1");
    expect(out).toContain(`data-kapi-id="${hash}"`);
    expect(out).toContain('data-kapi-loc="src/Page.tsx:1"');
    expect(() => parseSync(out as string, { syntax: "typescript", tsx: true })).not.toThrow();
  });

  it("stamps attribute-only elements with data-kapi-attr", () => {
    const out = rt('<input placeholder="Search..." />');
    const hash = hashKey("Search...", "input[placeholder]");
    expect(out).toContain(`data-kapi-attr="placeholder:${hash}"`);
    expect(out).not.toContain("data-kapi-id=");
    expect(() => parseSync(out as string, { syntax: "typescript", tsx: true })).not.toThrow();
  });

  it("stamps both when an element has content and attributes", () => {
    const out = rt('<button aria-label="Close dialog">Save</button>');
    expect(out).toContain(`data-kapi-id="${hashKey("Save", "button")}"`);
    expect(out).toContain(
      `data-kapi-attr="aria-label:${hashKey("Close dialog", "button[aria-label]")}"`,
    );
  });

  it("emits nothing extra when review is off", () => {
    const out = transform("<h1>Welcome back</h1>", "src/Page.tsx", { mode: "runtime" })?.code;
    expect(out).not.toContain("data-kapi-");
  });

  it("keeps ops disjoint with rich inline content", () => {
    const out = rt('<p>Click <a href="/x">here</a> to continue.</p>');
    expect(out).toContain("data-kapi-id=");
    expect(() => parseSync(out as string, { syntax: "typescript", tsx: true })).not.toThrow();
  });
});

// ─── Store + handler ─────────────────────────────────────────

const SOURCE = "<h1>Welcome back</h1>";

function seedKbfTree(dir: string): string {
  const doc = extractDocument(SOURCE, { filename: "src/Page.tsx" })!;
  const hash = doc.blocks[0].hash;
  doc.blocks[0].targets = { de: [{ text: "Willkommen zurück" }] } as never;
  mkdirSync(join(dir, "i18n", "src"), { recursive: true });
  writeFileSync(
    join(dir, "i18n", "src", "Page.kbf.json"),
    JSON.stringify({
      schemaVersion: "1.0",
      kind: "kapi-bundle",
      generator: { id: "@neokapi/i18n-react", version: "0.0.0" },
      project: { id: "test", sourceLocale: "en" },
      documents: [doc],
    }),
  );
  // A stand-off term annotation, as kapi term-check would produce.
  writeFileSync(
    join(dir, "i18n", "term-check.overlays.jsonl"),
    [
      JSON.stringify({
        type: "header",
        annotationType: "@neokapi/term-detector",
        annotationVersion: "1",
        producer: { id: "kapi", version: "0" },
        created: "2026-07-11T00:00:00Z",
        targetArchive: "x",
      }),
      JSON.stringify({
        type: "annotation",
        id: "a1",
        block: hash,
        anchor: { kind: "block" },
        data: { term: "Willkommen", note: "brand greeting" },
      }),
    ].join("\n"),
  );
  return hash;
}

describe("ReviewStore", () => {
  it("serves payloads with flattened source/targets and annotations", () => {
    const dir = scratch();
    const hash = seedKbfTree(dir);
    const store = new ReviewStore(join(dir, "i18n"));

    const payload = store.get(hash)!;
    expect(payload.sourceText).toBe("Welcome back");
    expect(payload.targets.de.text).toBe("Willkommen zurück");
    expect(payload.properties.element).toBe("h1");
    expect(payload.annotations).toHaveLength(1);
    expect(payload.annotations[0].annotationType).toBe("@neokapi/term-detector");
  });

  it("writes target edits back into the .kbf.json and broadcasts", () => {
    const dir = scratch();
    const hash = seedKbfTree(dir);
    const store = new ReviewStore(join(dir, "i18n"));
    const events: unknown[] = [];
    store.subscribe((u) => events.push(u));

    const updated = store.put(hash, "de", "Willkommen zurück!")!;
    expect(updated.targets.de.text).toBe("Willkommen zurück!");
    expect(events).toEqual([{ hash, locale: "de", text: "Willkommen zurück!" }]);

    const onDisk = JSON.parse(readFileSync(join(dir, "i18n", "src", "Page.kbf.json"), "utf-8"));
    expect(onDisk.documents[0].blocks[0].targets.de[0].text).toBe("Willkommen zurück!");
  });

  it("returns null for unknown hashes", () => {
    const dir = scratch();
    seedKbfTree(dir);
    const store = new ReviewStore(join(dir, "i18n"));
    expect(store.get("nope")).toBeNull();
    expect(store.put("nope", "de", "x")).toBeNull();
  });

  it("picks up external .kbf.json edits via mtime refresh", async () => {
    const dir = scratch();
    const hash = seedKbfTree(dir);
    const store = new ReviewStore(join(dir, "i18n"));
    expect(store.get(hash)!.targets.de.text).toBe("Willkommen zurück");

    // Simulate `kapi translate` rewriting the file.
    const path = join(dir, "i18n", "src", "Page.kbf.json");
    const raw = JSON.parse(readFileSync(path, "utf-8"));
    raw.documents[0].blocks[0].targets.de = [{ text: "extern" }];
    await new Promise((r) => setTimeout(r, 5)); // ensure mtime moves
    writeFileSync(path, JSON.stringify(raw));

    expect(store.get(hash)!.targets.de.text).toBe("extern");
  });
});

describe("handleReviewRequest", () => {
  function fakeRes() {
    const res = {
      statusCode: 0,
      body: "",
      headers: {} as Record<string, unknown>,
      writeHead(status: number, headers?: Record<string, unknown>) {
        this.statusCode = status;
        Object.assign(this.headers, headers);
        return this;
      },
      write(chunk: string) {
        this.body += chunk;
        return true;
      },
      end(chunk?: string) {
        if (chunk) this.body += chunk;
      },
    };
    return res;
  }

  it("GET {hash} returns the payload; unmatched routes fall through", () => {
    const dir = scratch();
    const hash = seedKbfTree(dir);
    const store = new ReviewStore(join(dir, "i18n"));

    const res = fakeRes();
    const handled = handleReviewRequest(
      store,
      { method: "GET" } as never,
      res as never,
      `/${hash}`,
    );
    expect(handled).toBe(true);
    expect(res.statusCode).toBe(200);
    expect(JSON.parse(res.body).sourceText).toBe("Welcome back");

    const res2 = fakeRes();
    expect(handleReviewRequest(store, { method: "POST" } as never, res2 as never, `/${hash}`)).toBe(
      false,
    );
  });

  it("GET annotations returns the by-hash map", () => {
    const dir = scratch();
    const hash = seedKbfTree(dir);
    const store = new ReviewStore(join(dir, "i18n"));
    const res = fakeRes();
    handleReviewRequest(store, { method: "GET" } as never, res as never, "/annotations");
    const map = JSON.parse(res.body);
    expect(map[hash]).toHaveLength(1);
  });
});
