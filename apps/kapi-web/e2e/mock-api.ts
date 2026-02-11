/**
 * REST API mock for kapi-web E2E tests.
 *
 * Intercepts fetch calls to /api/v1/* and returns realistic mock data.
 * Uses Playwright page.route() for network-level mocking.
 */
import type { Page, Route } from "@playwright/test";

// ---------------------------------------------------------------------------
// In-memory stores
// ---------------------------------------------------------------------------

interface ProjectItem {
  name: string;
  format: string;
  type: string;
  size: number;
  block_count: number;
  word_count: number;
}

interface Project {
  id: string;
  name: string;
  source_locale: string;
  target_locales: string[];
  items: ProjectItem[];
  created_at: string;
  modified_at: string;
}

interface Block {
  id: string;
  source: string;
  source_coded: string;
  targets: Record<string, string>;
  targets_coded: Record<string, string>;
  translatable: boolean;
  has_spans: boolean;
  properties: Record<string, string>;
}

interface TMEntry {
  id: string;
  source: string;
  target: string;
  source_locale: string;
  target_locale: string;
  updated_at: string;
}

interface TermInfo {
  text: string;
  locale: string;
  status: string;
}

interface Concept {
  id: string;
  domain: string;
  definition: string;
  terms: TermInfo[];
  created_at: string;
  updated_at: string;
}

let nextId = 1;
function genId(): string {
  return `mock-${nextId++}`;
}

let projects: Project[] = [];
let projectBlocks: Record<string, Record<string, Block[]>> = {};
let tmEntries: TMEntry[] = [];
let concepts: Concept[] = [];

function reset() {
  nextId = 1;
  projects = [];
  projectBlocks = {};
  tmEntries = [];
  concepts = [];
}

function guessFormat(fileName: string): string {
  const ext = fileName.substring(fileName.lastIndexOf(".")).toLowerCase();
  const map: Record<string, string> = {
    ".html": "html", ".htm": "html", ".json": "json", ".yaml": "yaml",
    ".yml": "yaml", ".md": "markdown", ".xml": "xml", ".po": "po",
    ".xlf": "xliff", ".xliff": "xliff", ".properties": "properties",
    ".srt": "srt", ".vtt": "vtt", ".csv": "csv", ".txt": "plaintext",
  };
  return map[ext] || "plaintext";
}

function sampleBlocks(_sourceLocale: string, _targetLocales: string[]): Block[] {
  const sources = [
    "Welcome to our website",
    "About Us",
    "Contact our team for more information",
    "Read our latest blog post",
    "Sign up for our newsletter",
    "Privacy Policy",
    "Terms of Service",
    "Copyright 2025. All rights reserved.",
  ];
  return sources.map((src, i) => ({
    id: `b${i + 1}`,
    source: src,
    source_coded: src,
    targets: {},
    targets_coded: {},
    translatable: true,
    has_spans: false,
    properties: {},
  }));
}

function makeItem(fileName: string): ProjectItem {
  return {
    name: fileName,
    format: guessFormat(fileName),
    type: "file",
    size: 1024,
    block_count: 8,
    word_count: 42,
  };
}

// ---------------------------------------------------------------------------
// Route helpers
// ---------------------------------------------------------------------------

function json(route: Route, data: unknown, status = 200) {
  route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
}

function extractPathSegment(url: string, afterKey: string): string {
  const parts = new URL(url).pathname.split("/");
  const idx = parts.indexOf(afterKey);
  return idx >= 0 && idx + 1 < parts.length ? parts[idx + 1] : "";
}

// ---------------------------------------------------------------------------
// Route setup
// ---------------------------------------------------------------------------

export async function setupMockApi(page: Page) {
  reset();

  // Config — local mode
  await page.route("**/api/v1/config", (route) => {
    json(route, { mode: "local" });
  });

  // Locales
  await page.route("**/api/v1/locales", (route) => {
    json(route, [
      { code: "en", display_name: "English" }, { code: "fr", display_name: "French" },
      { code: "de", display_name: "German" }, { code: "es", display_name: "Spanish" },
      { code: "ja", display_name: "Japanese" }, { code: "zh-CN", display_name: "Chinese (Simplified)" },
      { code: "ko", display_name: "Korean" }, { code: "pt-BR", display_name: "Portuguese (Brazil)" },
    ]);
  });

  // Formats + tools
  await page.route("**/api/v1/formats", (route) => json(route, []));
  await page.route("**/api/v1/tools", (route) => json(route, []));

  // Providers
  await page.route("**/api/v1/workspaces/*/providers", (route) => json(route, []));
  await page.route("**/api/v1/workspaces/*/providers/**", (route) => json(route, { ok: true }));

  // ---------------------------------------------------------------------------
  // Projects
  // ---------------------------------------------------------------------------

  // List / create projects
  await page.route(/\/api\/v1\/workspaces\/[^/]+\/editor\/projects(\?|$)/, (route, request) => {
    if (request.method() === "POST") {
      const body = request.postDataJSON();
      const p: Project = {
        id: genId(), name: body.name,
        source_locale: body.source_locale, target_locales: body.target_locales,
        items: [], created_at: new Date().toISOString(), modified_at: new Date().toISOString(),
      };
      projects.push(p);
      projectBlocks[p.id] = {};
      json(route, p);
    } else {
      json(route, projects);
    }
  });

  // Export (must be before generic files/*)
  await page.route(/\/files\/[^/]+\/export(\?|$)/, (route) => {
    route.fulfill({
      status: 200,
      contentType: "application/octet-stream",
      body: Buffer.from("<html><body>Exported</body></html>"),
    });
  });

  // Pseudo-translate
  await page.route(/\/files\/[^/]+\/pseudo(\?|$)/, async (route) => {
    const url = route.request().url();
    const pid = extractPathSegment(url, "projects");
    const parts = new URL(url).pathname.split("/");
    const pseudoIdx = parts.indexOf("pseudo");
    const fileName = parts[pseudoIdx - 1];
    const blocks = projectBlocks[pid]?.[fileName] || [];
    let body: Record<string, string> = {};
    try { body = route.request().postDataJSON(); } catch { /* ignore */ }
    const tl = body.target_locale || new URL(url).searchParams.get("target_locale") || "fr";
    let translated = 0;
    for (const b of blocks) {
      if (b.translatable) {
        b.targets[tl] = `[${b.source}]`;
        b.targets_coded[tl] = `[${b.source_coded}]`;
        b.properties["translation-origin"] = "pseudo";
        translated++;
      }
    }
    json(route, { total_blocks: blocks.length, translated_blocks: translated, word_count: 42 });
  });

  // AI translate
  await page.route(/\/files\/[^/]+\/ai-translate(\?|$)/, async (route) => {
    const url = route.request().url();
    const pid = extractPathSegment(url, "projects");
    const parts = new URL(url).pathname.split("/");
    const aiIdx = parts.indexOf("ai-translate");
    const fileName = parts[aiIdx - 1];
    const blocks = projectBlocks[pid]?.[fileName] || [];
    let body: Record<string, string> = {};
    try { body = route.request().postDataJSON(); } catch { /* ignore */ }
    const tl = body.target_locale || new URL(url).searchParams.get("target_locale") || "fr";
    let translated = 0;
    for (const b of blocks) {
      if (b.translatable) {
        b.targets[tl] = `AI: ${b.source}`;
        b.targets_coded[tl] = `AI: ${b.source_coded}`;
        b.properties["translation-origin"] = "machine";
        translated++;
      }
    }
    json(route, { total_blocks: blocks.length, translated_blocks: translated, word_count: 42 });
  });

  // TM translate
  await page.route(/\/files\/[^/]+\/tm-translate(\?|$)/, (route) => {
    const url = route.request().url();
    const pid = extractPathSegment(url, "projects");
    const parts = new URL(url).pathname.split("/");
    const tmIdx = parts.indexOf("tm-translate");
    const fileName = parts[tmIdx - 1];
    const blocks = projectBlocks[pid]?.[fileName] || [];
    json(route, blocks);
  });

  // Word count
  await page.route(/\/files\/[^/]+\/wordcount(\?|$)/, (route) => {
    json(route, { source_words: 42, source_chars: 210, target_words: {}, target_chars: {} });
  });

  // Get blocks for file
  await page.route(/\/files\/[^/]+\/blocks(\?|$)/, (route) => {
    const url = route.request().url();
    const pid = extractPathSegment(url, "projects");
    const parts = new URL(url).pathname.split("/");
    const blocksIdx = parts.indexOf("blocks");
    const fileName = parts[blocksIdx - 1];
    const blocks = projectBlocks[pid]?.[fileName] || [];
    json(route, blocks);
  });

  // Upload files (POST to /files)
  await page.route(/\/editor\/projects\/[^/]+\/files(\?|$)/, (route, request) => {
    if (request.method() === "POST") {
      const pid = extractPathSegment(request.url(), "projects");
      const p = projects.find((p) => p.id === pid);
      if (p) {
        const fileName = `file-${p.items.length + 1}.html`;
        p.items.push(makeItem(fileName));
        projectBlocks[p.id][fileName] = sampleBlocks(p.source_locale, p.target_locales);
        p.modified_at = new Date().toISOString();
        json(route, p);
      } else {
        json(route, { error: "not found" }, 404);
      }
    } else {
      route.continue();
    }
  });

  // Remove file
  await page.route(/\/editor\/projects\/[^/]+\/files\/[^/]+(\?|$)/, (route, request) => {
    if (request.method() === "DELETE") {
      const pid = extractPathSegment(request.url(), "projects");
      const parts = new URL(request.url()).pathname.split("/");
      const fileName = parts[parts.length - 1];
      const p = projects.find((p) => p.id === pid);
      if (p) {
        p.items = p.items.filter((i) => i.name !== fileName);
        if (projectBlocks[p.id]) delete projectBlocks[p.id][fileName];
        json(route, p);
      } else {
        json(route, { error: "not found" }, 404);
      }
    } else {
      route.continue();
    }
  });

  // TM/term lookup for blocks
  await page.route(/\/blocks\/[^/]+\/tm-lookup(\?|$)/, (route) => {
    json(route, { matches: [] });
  });
  await page.route(/\/blocks\/[^/]+\/term-lookup(\?|$)/, (route) => {
    json(route, { matches: [] });
  });

  // Update block
  await page.route(/\/editor\/projects\/[^/]+\/blocks\/[^/]+(\?|$)/, (route, request) => {
    if (request.method() === "PUT") {
      const body = request.postDataJSON();
      const pid = extractPathSegment(request.url(), "projects");
      const tl = new URL(request.url()).searchParams.get("target_locale") || "fr";
      const parts = new URL(request.url()).pathname.split("/");
      const blockId = parts[parts.length - 1];
      const fileBlocks = projectBlocks[pid];
      if (fileBlocks) {
        for (const blocks of Object.values(fileBlocks)) {
          const block = blocks.find((b) => b.id === blockId);
          if (block) {
            if (body.target) {
              block.targets[tl] = body.target;
              block.targets_coded[tl] = body.target_coded || body.target;
              block.properties["translation-status"] = "translated";
            }
            json(route, block);
            return;
          }
        }
      }
      json(route, { error: "not found" }, 404);
    } else {
      route.continue();
    }
  });

  // Get / delete project (must be after more specific sub-resource routes)
  await page.route(/\/editor\/projects\/[^/]+(\?|$)/, (route, request) => {
    const pid = extractPathSegment(request.url(), "projects");
    if (request.method() === "DELETE") {
      projects = projects.filter((p) => p.id !== pid);
      json(route, { ok: true });
    } else {
      const p = projects.find((p) => p.id === pid);
      if (p) json(route, p);
      else json(route, { error: "not found" }, 404);
    }
  });

  // ---------------------------------------------------------------------------
  // Translation Memory
  // ---------------------------------------------------------------------------

  // TM count (must be before generic /tm/*)
  await page.route(/\/workspaces\/[^/]+\/tm\/count(\?|$)/, (route) => {
    json(route, { count: tmEntries.length });
  });

  // TM update/delete
  await page.route(/\/workspaces\/[^/]+\/tm\/[^/]+(\?|$)/, (route, request) => {
    const parts = new URL(request.url()).pathname.split("/");
    const entryId = parts[parts.length - 1];
    if (entryId === "count") { route.continue(); return; }

    if (request.method() === "PUT") {
      const body = request.postDataJSON();
      const entry = tmEntries.find((e) => e.id === entryId);
      if (entry) {
        entry.target = body.target || entry.target;
        entry.updated_at = new Date().toISOString();
        json(route, entry);
      } else {
        json(route, { error: "not found" }, 404);
      }
    } else if (request.method() === "DELETE") {
      tmEntries = tmEntries.filter((e) => e.id !== entryId);
      json(route, { ok: true });
    } else {
      route.continue();
    }
  });

  // TM list / create
  await page.route(/\/workspaces\/[^/]+\/tm(\?|$)/, (route, request) => {
    if (request.method() === "POST") {
      const body = request.postDataJSON();
      const entry: TMEntry = {
        id: genId(), source: body.source, target: body.target,
        source_locale: body.source_locale, target_locale: body.target_locale,
        updated_at: new Date().toISOString(),
      };
      tmEntries.push(entry);
      json(route, entry);
      return;
    }

    const url = new URL(request.url());
    const q = url.searchParams.get("q") || "";
    const srcLocale = url.searchParams.get("source_locale") || "";
    const tgtLocale = url.searchParams.get("target_locale") || "";
    const offset = parseInt(url.searchParams.get("offset") || "0");
    const limit = parseInt(url.searchParams.get("limit") || "50");

    let filtered = tmEntries;
    if (q) filtered = filtered.filter((e) => e.source.includes(q) || e.target.includes(q));
    if (srcLocale) filtered = filtered.filter((e) => e.source_locale === srcLocale);
    if (tgtLocale) filtered = filtered.filter((e) => e.target_locale === tgtLocale);

    json(route, {
      entries: filtered.slice(offset, offset + limit),
      total_count: filtered.length,
    });
  });

  // ---------------------------------------------------------------------------
  // Terminology
  // ---------------------------------------------------------------------------

  // Terms count
  await page.route(/\/workspaces\/[^/]+\/terms\/count(\?|$)/, (route) => {
    json(route, { count: concepts.length });
  });

  // Terms import/export
  await page.route(/\/terms\/import\//, (route) => json(route, { imported: 0 }));
  await page.route(/\/terms\/export\/json(\?|$)/, (route) => json(route, concepts));

  // Terms update/delete
  await page.route(/\/workspaces\/[^/]+\/terms\/[^/]+(\?|$)/, (route, request) => {
    const parts = new URL(request.url()).pathname.split("/");
    const conceptId = parts[parts.length - 1];
    if (["count", "import", "export"].includes(conceptId)) { route.continue(); return; }

    if (request.method() === "PUT") {
      const body = request.postDataJSON();
      const concept = concepts.find((c) => c.id === conceptId);
      if (concept) {
        if (body.domain !== undefined) concept.domain = body.domain;
        if (body.definition !== undefined) concept.definition = body.definition;
        if (body.terms) concept.terms = body.terms;
        concept.updated_at = new Date().toISOString();
        json(route, concept);
      } else {
        json(route, { error: "not found" }, 404);
      }
    } else if (request.method() === "DELETE") {
      concepts = concepts.filter((c) => c.id !== conceptId);
      json(route, { ok: true });
    } else {
      route.continue();
    }
  });

  // Terms list / create
  await page.route(/\/workspaces\/[^/]+\/terms(\?|$)/, (route, request) => {
    if (request.method() === "POST") {
      const body = request.postDataJSON();
      const now = new Date().toISOString();
      const concept: Concept = {
        id: genId(),
        domain: body.domain || "",
        definition: body.definition || "",
        terms: body.terms || [],
        created_at: now,
        updated_at: now,
      };
      concepts.push(concept);
      json(route, concept);
      return;
    }

    const url = new URL(request.url());
    const q = url.searchParams.get("q") || "";
    const srcLocale = url.searchParams.get("source_locale") || "";
    const tgtLocale = url.searchParams.get("target_locale") || "";
    const offset = parseInt(url.searchParams.get("offset") || "0");
    const limit = parseInt(url.searchParams.get("limit") || "50");

    let filtered = concepts;
    if (q) filtered = filtered.filter((c) => c.terms.some((t) => t.text.includes(q)) || c.domain.includes(q));
    if (srcLocale) filtered = filtered.filter((c) => c.terms.some((t) => t.locale === srcLocale));
    if (tgtLocale) filtered = filtered.filter((c) => c.terms.some((t) => t.locale === tgtLocale));

    json(route, {
      concepts: filtered.slice(offset, offset + limit),
      total_count: filtered.length,
    });
  });
}

// ---------------------------------------------------------------------------
// Seeding helpers (call after setupMockApi, before page.goto)
// ---------------------------------------------------------------------------

export function seedTMEntries() {
  const entries: Omit<TMEntry, "id" | "updated_at">[] = [
    { source: "Welcome to our website", target: "Bienvenue sur notre site", source_locale: "en", target_locale: "fr" },
    { source: "About Us", target: "\u00C0 propos de nous", source_locale: "en", target_locale: "fr" },
    { source: "Contact our team", target: "Contactez notre \u00E9quipe", source_locale: "en", target_locale: "fr" },
    { source: "Read our blog", target: "Lisez notre blog", source_locale: "en", target_locale: "fr" },
    { source: "Privacy Policy", target: "Politique de confidentialit\u00E9", source_locale: "en", target_locale: "fr" },
  ];
  for (const e of entries) {
    tmEntries.push({ ...e, id: genId(), updated_at: new Date().toISOString() });
  }
}

export function seedConcepts() {
  const defs: Omit<Concept, "id" | "created_at" | "updated_at">[] = [
    { domain: "UI", definition: "Main overview page", terms: [
      { text: "dashboard", locale: "en", status: "preferred" },
      { text: "tableau de bord", locale: "fr", status: "preferred" },
    ] },
    { domain: "UI", definition: "Authentication action", terms: [
      { text: "login", locale: "en", status: "approved" },
      { text: "connexion", locale: "fr", status: "approved" },
    ] },
    { domain: "UI", definition: "Configuration page", terms: [
      { text: "settings", locale: "en", status: "preferred" },
      { text: "param\u00E8tres", locale: "fr", status: "preferred" },
    ] },
    { domain: "platform", definition: "Collaboration space", terms: [
      { text: "workspace", locale: "en", status: "approved" },
      { text: "espace de travail", locale: "fr", status: "approved" },
    ] },
    { domain: "localization", definition: "Database of previous translations", terms: [
      { text: "translation memory", locale: "en", status: "preferred" },
      { text: "m\u00E9moire de traduction", locale: "fr", status: "preferred" },
    ] },
  ];
  const now = new Date().toISOString();
  for (const d of defs) {
    concepts.push({ ...d, id: genId(), created_at: now, updated_at: now });
  }
}

export function seedProject(name: string, sourceLocale: string, targetLocales: string[], fileNames: string[]) {
  const items: ProjectItem[] = fileNames.map(makeItem);
  const p: Project = {
    id: genId(), name,
    source_locale: sourceLocale, target_locales: targetLocales,
    items, created_at: new Date().toISOString(), modified_at: new Date().toISOString(),
  };
  projects.push(p);
  projectBlocks[p.id] = {};
  for (const f of fileNames) {
    projectBlocks[p.id][f] = sampleBlocks(sourceLocale, targetLocales);
  }
  return p;
}
