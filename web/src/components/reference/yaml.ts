// Value/default helpers and a minimal YAML serializer for the reference pages.
// The form holds plain JS values; this module seeds them from schema defaults
// and renders the current (non-default) values as a copyable YAML block.

import type { ComponentSchema, PropertySchema } from "@neokapi/reference-data";

/** Seed form values from a schema's property defaults. */
export function seedDefaults(schema: ComponentSchema | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  const props = schema?.properties ?? {};
  for (const [key, prop] of Object.entries(props)) {
    out[key] = defaultFor(prop);
  }
  return out;
}

function defaultFor(prop: PropertySchema): unknown {
  if (prop.default !== undefined) return prop.default;
  switch (prop.type) {
    case "boolean":
      return false;
    case "array":
      return [];
    case "object":
      return {};
    case "integer":
    case "number":
      return undefined;
    default:
      return "";
  }
}

function isEmpty(val: unknown): boolean {
  if (val === undefined || val === null || val === "") return true;
  if (Array.isArray(val)) return val.length === 0;
  if (typeof val === "object") return Object.keys(val as object).length === 0;
  return false;
}

function equalsDefault(prop: PropertySchema | undefined, val: unknown): boolean {
  if (!prop || prop.default === undefined) return false;
  return JSON.stringify(prop.default) === JSON.stringify(val);
}

/** PascalCase a format id: "html" → "Html", "openxml" → "Openxml". */
function pascalCase(s: string): string {
  return s
    .split(/[-_]/)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join("");
}

function scalarYaml(val: unknown): string {
  if (typeof val === "boolean") return String(val);
  if (typeof val === "number") return String(val);
  if (typeof val === "string") {
    // Quote strings that could be misread as YAML scalars.
    if (val === "" || /[:#{}[\],&*!|>'"%@`]/.test(val) || /^\s|\s$/.test(val)) {
      return JSON.stringify(val);
    }
    return val;
  }
  return JSON.stringify(val);
}

/**
 * One rendered YAML line, tagged with the top-level config key it belongs to
 * (null for structural/header lines), so the output panel can highlight the
 * lines a user has changed relative to the active baseline.
 */
export interface YamlLine {
  text: string;
  key: string | null;
}

function emitValue(
  key: string,
  val: unknown,
  indent: string,
  owner: string | null,
  lines: YamlLine[],
): void {
  if (Array.isArray(val)) {
    lines.push({ text: `${indent}${key}:`, key: owner });
    for (const item of val) {
      lines.push({ text: `${indent}  - ${scalarYaml(item)}`, key: owner });
    }
    return;
  }
  if (val && typeof val === "object") {
    lines.push({ text: `${indent}${key}:`, key: owner });
    for (const [k, v] of Object.entries(val as Record<string, unknown>)) {
      if (isEmpty(v)) continue;
      emitValue(k, v, `${indent}  `, owner, lines);
    }
    return;
  }
  lines.push({ text: `${indent}${key}: ${scalarYaml(val)}`, key: owner });
}

/** Join rendered YAML lines into a copyable string. */
export function yamlText(lines: YamlLine[]): string {
  return lines.map((l) => l.text).join("\n");
}

/**
 * Build a copyable YAML config for a format, as tagged lines. Only values that
 * differ from their schema default (and are non-empty) are emitted.
 */
export function buildFormatYamlLines(
  formatId: string,
  values: Record<string, unknown>,
  schema: ComponentSchema | undefined,
): YamlLine[] {
  const props = schema?.properties ?? {};
  const lines: YamlLine[] = [
    { text: "apiVersion: neokapi/v1", key: null },
    { text: `kind: ${pascalCase(formatId)}FormatConfig`, key: null },
    { text: "metadata:", key: null },
    { text: `  name: ${formatId}`, key: null },
    { text: "spec:", key: null },
  ];
  const entries = changedEntries(values, props);
  if (entries.length === 0) {
    lines.push({ text: "  # (default configuration)", key: null });
    return lines;
  }
  for (const [key, val] of entries) emitValue(key, val, "  ", key, lines);
  return lines;
}

/**
 * Build a copyable YAML config block for a tool — a flat mapping of the
 * non-default parameter values, matching how tool configs appear inline in a
 * flow step.
 */
export function buildToolYamlLines(
  values: Record<string, unknown>,
  schema: ComponentSchema | undefined,
): YamlLine[] {
  const props = schema?.properties ?? {};
  const entries = changedEntries(values, props);
  if (entries.length === 0) return [{ text: "# (default configuration)", key: null }];
  const lines: YamlLine[] = [];
  for (const [key, val] of entries) emitValue(key, val, "", key, lines);
  return lines;
}

function changedEntries(
  values: Record<string, unknown>,
  props: Record<string, PropertySchema>,
): [string, unknown][] {
  return Object.entries(values)
    .filter(([key, val]) => {
      const prop = props[key];
      if (!prop) return false;
      if (isEmpty(val)) return false;
      if (equalsDefault(prop, val)) return false;
      return true;
    })
    .sort(([a], [b]) => a.localeCompare(b));
}
