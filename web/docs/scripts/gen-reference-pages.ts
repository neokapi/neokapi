#!/usr/bin/env node
// gen-reference-pages.ts — R4 (#673): generate one static MDX page per command,
// format, and tool from the @neokapi/reference-data dataset.
//
// HARD RULE: pages are generated from the data, never hand-authored. Each file
// carries a "GENERATED — do not edit" header. The MDX body is intentionally
// thin: frontmatter (for SEO title/description + slug) + an <h1> + a single
// React component (CommandReferencePage / FormatReferencePage /
// ToolReferencePage) that renders ALL fields from the same data at build time.
// Change the data (rerun `make generate-reference-docs`) or the page component,
// then regenerate here.
//
// Output:
//   web/docs/docs/reference/commands/<slug>.mdx
//   web/docs/docs/reference/formats/<slug>.mdx
//   web/docs/docs/reference/tools/<slug>.mdx
//
// Routes (docs routeBasePath is "/"): /reference/<kind>/<slug>.
//
// Deterministic + idempotent: entries are processed in id order, the target
// dirs are wiped (except the kept index.mdx is NOT in these subdirs) and fully
// rewritten each run, so two runs from the same data produce byte-identical
// output. Run via `vp run build` / `vp run start` (prebuild/prestart hooks) or
// `make generate-reference-pages`.
//
// Usage: node --experimental-strip-types web/docs/scripts/gen-reference-pages.ts

import { mkdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import commandsJson from "@neokapi/reference-data/data/commands.json" with { type: "json" };
import formatsJson from "@neokapi/reference-data/data/formats.json" with { type: "json" };
import toolsJson from "@neokapi/reference-data/data/tools.json" with { type: "json" };
import type {
  CommandDataset,
  CommandEntry,
  ReferenceDataset,
  ReferenceEntry,
} from "@neokapi/reference-data";

import {
  builtinToolIds,
  commandSlug,
  formatSlug,
  toolSlug,
} from "../src/components/reference/slugs.ts";

const commands = commandsJson as unknown as CommandDataset;
const formats = formatsJson as unknown as ReferenceDataset;
const tools = toolsJson as unknown as ReferenceDataset;

const __dirname = dirname(fileURLToPath(import.meta.url));
const DOCS_ROOT = resolve(__dirname, "..");
const OUT_ROOT = join(DOCS_ROOT, "docs", "reference");

const HEADER = `{/*
  GENERATED FILE — DO NOT EDIT.
  Produced by web/docs/scripts/gen-reference-pages.ts from @neokapi/reference-data.
  To change this page, edit the data (regenerate with \`make generate-reference-docs\`)
  or the page component, then rerun \`make generate-reference-pages\`.
*/}`;

// ── Helpers ──────────────────────────────────────────────────────────────────

/** Collapse whitespace and strip markdown emphasis for a frontmatter value. */
function firstLine(text: string | undefined): string {
  if (!text) return "";
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    return line.replace(/\*\*|__|`/g, "");
  }
  return "";
}

/** Make a string safe for a single-quoted YAML frontmatter scalar. */
function yamlScalar(s: string): string {
  const collapsed = s.replace(/\s+/g, " ").trim();
  // Single-quote and escape embedded single quotes by doubling them.
  return `'${collapsed.replace(/'/g, "''")}'`;
}

/** Trim a description to a sensible meta length (~155 chars) at a word break. */
function clampDescription(s: string, max = 160): string {
  const t = s.replace(/\s+/g, " ").trim();
  if (t.length <= max) return t;
  const cut = t.slice(0, max);
  const lastSpace = cut.lastIndexOf(" ");
  return (lastSpace > 40 ? cut.slice(0, lastSpace) : cut).replace(/[.,;:]$/, "") + "…";
}

interface PageSpec {
  /** Output file path. */
  file: string;
  /** Full MDX contents. */
  body: string;
}

function frontmatter(fields: Record<string, string>): string {
  const lines = Object.entries(fields).map(([k, v]) => `${k}: ${v}`);
  return `---\n${lines.join("\n")}\n---`;
}

// ── Command pages ────────────────────────────────────────────────────────────

function commandPage(cmd: CommandEntry): PageSpec {
  const name = cmd.path.join(" ");
  const slug = commandSlug(cmd);
  const desc = clampDescription(cmd.short || firstLine(cmd.long) || `The kapi ${name} command.`);
  const title = `kapi ${name}`;

  const fm = frontmatter({
    id: slug,
    title: yamlScalar(title),
    description: yamlScalar(desc),
    sidebar_label: yamlScalar(name),
    slug: `/reference/commands/${slug}`,
    hide_table_of_contents: "true",
  });

  const body = [
    fm,
    "",
    HEADER,
    "",
    `import CommandReferencePage from "@site/src/components/reference/pages/CommandReferencePage";`,
    "",
    `# kapi ${name}`,
    "",
    `<CommandReferencePage id={${JSON.stringify(cmd.id)}} />`,
    "",
  ].join("\n");

  return { file: join(OUT_ROOT, "commands", `${slug}.mdx`), body };
}

// ── Format pages ─────────────────────────────────────────────────────────────

function formatPage(entry: ReferenceEntry): PageSpec {
  const slug = formatSlug(entry);
  const desc = clampDescription(
    entry.description ||
      firstLine(entry.doc?.overview) ||
      `The ${entry.displayName} data format in neokapi.`,
  );
  const exts = entry.extensions?.length ? ` (${entry.extensions.join(", ")})` : "";
  const title = `${entry.displayName} format`;

  const fm = frontmatter({
    id: slug,
    title: yamlScalar(title),
    description: yamlScalar(desc),
    sidebar_label: yamlScalar(entry.displayName),
    slug: `/reference/formats/${slug}`,
    hide_table_of_contents: "true",
  });

  const body = [
    fm,
    "",
    HEADER,
    "",
    `import FormatReferencePage from "@site/src/components/reference/pages/FormatReferencePage";`,
    "",
    `# ${entry.displayName} format${exts}`,
    "",
    `<FormatReferencePage id={${JSON.stringify(entry.id)}} />`,
    "",
  ].join("\n");

  return { file: join(OUT_ROOT, "formats", `${slug}.mdx`), body };
}

// ── Tool pages ───────────────────────────────────────────────────────────────

function toolPage(entry: ReferenceEntry, builtins: ReadonlySet<string>): PageSpec {
  const slug = toolSlug(entry, builtins);
  const desc = clampDescription(
    entry.description ||
      firstLine(entry.doc?.overview) ||
      `The ${entry.displayName} processing tool in neokapi.`,
  );
  const title = `${entry.displayName} tool`;

  const fm = frontmatter({
    id: slug,
    title: yamlScalar(title),
    description: yamlScalar(desc),
    sidebar_label: yamlScalar(entry.displayName),
    slug: `/reference/tools/${slug}`,
    hide_table_of_contents: "true",
  });

  const body = [
    fm,
    "",
    HEADER,
    "",
    `import ToolReferencePage from "@site/src/components/reference/pages/ToolReferencePage";`,
    "",
    `# ${entry.displayName} tool`,
    "",
    `<ToolReferencePage id={${JSON.stringify(entry.id)}} source={${JSON.stringify(entry.source)}} />`,
    "",
  ].join("\n");

  return { file: join(OUT_ROOT, "tools", `${slug}.mdx`), body };
}

// ── Main ─────────────────────────────────────────────────────────────────────

function main(): void {
  const specs: PageSpec[] = [];

  // Deterministic order: by id (commands), by id (formats), by source+id (tools).
  const sortedCommands = [...commands.commands].sort((a, b) => a.id.localeCompare(b.id));
  for (const cmd of sortedCommands) specs.push(commandPage(cmd));

  const sortedFormats = [...formats.entries].sort((a, b) => a.id.localeCompare(b.id));
  for (const f of sortedFormats) specs.push(formatPage(f));

  const builtins = builtinToolIds(tools.entries);
  const sortedTools = [...tools.entries].sort(
    (a, b) => a.source.localeCompare(b.source) || a.id.localeCompare(b.id),
  );
  for (const t of sortedTools) specs.push(toolPage(t, builtins));

  // Collision guard: slugs must be unique per kind.
  const seen = new Map<string, string>();
  for (const s of specs) {
    const prev = seen.get(s.file);
    if (prev) {
      throw new Error(`Slug collision: ${s.file} produced twice. Fix the slug helper.`);
    }
    seen.set(s.file, s.file);
  }

  // Wipe + rewrite each subdir so removed/renamed entries don't leave stale
  // pages. The kept reference/index.mdx lives in OUT_ROOT (not a subdir), so it
  // is untouched.
  for (const sub of ["commands", "formats", "tools"]) {
    const dir = join(OUT_ROOT, sub);
    rmSync(dir, { recursive: true, force: true });
    mkdirSync(dir, { recursive: true });
  }

  for (const s of specs) {
    writeFileSync(s.file, s.body, "utf8");
  }

  const counts = {
    commands: sortedCommands.length,
    formats: sortedFormats.length,
    tools: sortedTools.length,
  };
  console.log(
    `gen-reference-pages: wrote ${specs.length} pages ` +
      `(${counts.commands} commands, ${counts.formats} formats, ${counts.tools} tools) ` +
      `→ ${OUT_ROOT}/{commands,formats,tools}`,
  );
}

main();
