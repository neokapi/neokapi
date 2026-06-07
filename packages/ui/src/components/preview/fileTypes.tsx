import React from "react";
import {
  File,
  FileArchive,
  FileCode,
  FileImage,
  FileJson,
  FileSpreadsheet,
  FileText,
  FileType,
  Languages,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { cn } from "../../lib/utils";
import type { Lang } from "./highlight";

// fileTypes maps a filename's extension to the metadata the file explorer and
// output viewer need to render it consistently: an icon, a semantic accent
// colour, a short human label, the highlighter language, and a broad group. The
// accent palette is shared with the content-model "kind" colours so a learner
// builds one mental colour vocabulary across the whole lab (bilingual = green,
// markup = blue, catalog = violet, data = sky, …).

export type FileGroup =
  | "data" // structured config / messages (json, yaml, …)
  | "markup" // tag-based documents (html, xml, md)
  | "bilingual" // localisation exchange (xliff, tmx, po)
  | "catalog" // platform string catalogs (resx, arb, strings, …)
  | "doc" // office / binary documents (docx, xlsx, pdf)
  | "image"
  | "text";

export interface FileTypeInfo {
  /** Lower-case extension without the dot, or "" when there is none. */
  ext: string;
  /** Short human label, e.g. "JSON", "XLIFF", "Java properties". */
  label: string;
  /** Tailwind text-colour class for the icon + type accents. */
  colorClass: string;
  group: FileGroup;
  /** Highlighter language; "text" when no highlighting applies. */
  lang: Lang;
  /** Whether the bytes are binary (no useful text/source view). */
  binary: boolean;
  icon: LucideIcon;
}

// The accent vocabulary as Tailwind text-colour classes, shared with the
// content-model "kind" palette so a learner builds one colour vocabulary across
// the lab (bilingual = emerald, markup = blue, catalog = violet, data = sky, …).
const GROUP_CLASS: Record<FileGroup, string> = {
  data: "text-sky-600 dark:text-sky-400",
  markup: "text-blue-600 dark:text-blue-400",
  bilingual: "text-emerald-600 dark:text-emerald-400",
  catalog: "text-violet-600 dark:text-violet-400",
  doc: "text-amber-600 dark:text-amber-400",
  image: "text-pink-600 dark:text-pink-400",
  text: "text-slate-500 dark:text-slate-400",
};

interface Spec {
  label: string;
  group: FileGroup;
  lang: Lang;
  icon: LucideIcon;
  binary?: boolean;
}

// Extension table. Keep entries terse; unknown extensions fall back to plain
// text. Several l10n formats share an underlying syntax (xliff/tmx/sdlxliff are
// XML) — the lang drives highlighting, the group/label drive the icon + colour.
const TABLE: Record<string, Spec> = {
  // structured data
  json: { label: "JSON", group: "data", lang: "json", icon: FileJson },
  jsonc: { label: "JSON (comments)", group: "data", lang: "json", icon: FileJson },
  json5: { label: "JSON5", group: "data", lang: "json", icon: FileJson },
  yaml: { label: "YAML", group: "data", lang: "yaml", icon: FileCode },
  yml: { label: "YAML", group: "data", lang: "yaml", icon: FileCode },
  toml: { label: "TOML", group: "data", lang: "properties", icon: FileCode },
  ini: { label: "INI", group: "data", lang: "properties", icon: FileCode },
  properties: { label: "Java properties", group: "catalog", lang: "properties", icon: FileCode },
  csv: { label: "CSV", group: "data", lang: "csv", icon: FileSpreadsheet },
  tsv: { label: "TSV", group: "data", lang: "csv", icon: FileSpreadsheet },

  // markup / documents
  html: { label: "HTML", group: "markup", lang: "xml", icon: FileCode },
  htm: { label: "HTML", group: "markup", lang: "xml", icon: FileCode },
  xml: { label: "XML", group: "markup", lang: "xml", icon: FileCode },
  svg: { label: "SVG", group: "markup", lang: "xml", icon: FileImage },
  md: { label: "Markdown", group: "markup", lang: "markdown", icon: FileText },
  mdx: { label: "MDX", group: "markup", lang: "markdown", icon: FileText },
  markdown: { label: "Markdown", group: "markup", lang: "markdown", icon: FileText },

  // bilingual / localisation exchange
  xliff: { label: "XLIFF", group: "bilingual", lang: "xml", icon: Languages },
  xlf: { label: "XLIFF", group: "bilingual", lang: "xml", icon: Languages },
  sdlxliff: { label: "SDLXLIFF", group: "bilingual", lang: "xml", icon: Languages },
  mxliff: { label: "MXLIFF", group: "bilingual", lang: "xml", icon: Languages },
  tmx: { label: "TMX", group: "bilingual", lang: "xml", icon: Languages },
  tbx: { label: "TBX", group: "bilingual", lang: "xml", icon: Languages },
  po: { label: "Gettext PO", group: "bilingual", lang: "po", icon: Languages },
  pot: { label: "Gettext POT", group: "bilingual", lang: "po", icon: Languages },
  klf: { label: "KLF", group: "bilingual", lang: "json", icon: Languages },

  // platform string catalogs
  resx: { label: "RESX", group: "catalog", lang: "xml", icon: FileCode },
  arb: { label: "ARB", group: "catalog", lang: "json", icon: FileJson },
  strings: { label: "Apple .strings", group: "catalog", lang: "properties", icon: FileCode },
  stringsdict: { label: "Apple stringsdict", group: "catalog", lang: "xml", icon: FileCode },
  xcstrings: { label: "Xcode strings", group: "catalog", lang: "json", icon: FileJson },

  // office / binary docs
  docx: { label: "Word", group: "doc", lang: "text", icon: FileType, binary: true },
  xlsx: { label: "Excel", group: "doc", lang: "text", icon: FileSpreadsheet, binary: true },
  pptx: { label: "PowerPoint", group: "doc", lang: "text", icon: FileType, binary: true },
  pdf: { label: "PDF", group: "doc", lang: "text", icon: FileType, binary: true },
  zip: { label: "ZIP", group: "doc", lang: "text", icon: FileArchive, binary: true },
  klz: { label: "KLZ workspace", group: "doc", lang: "text", icon: FileArchive, binary: true },

  // images
  png: { label: "PNG", group: "image", lang: "text", icon: FileImage, binary: true },
  jpg: { label: "JPEG", group: "image", lang: "text", icon: FileImage, binary: true },
  jpeg: { label: "JPEG", group: "image", lang: "text", icon: FileImage, binary: true },
  gif: { label: "GIF", group: "image", lang: "text", icon: FileImage, binary: true },

  // plain text
  txt: { label: "Text", group: "text", lang: "text", icon: FileText },
  text: { label: "Text", group: "text", lang: "text", icon: FileText },
};

/** The lower-case extension of a filename, or "" when there is none. */
export function extOf(filename: string): string {
  const base = filename.replace(/\/+$/, "").split("/").pop() ?? filename;
  const dot = base.lastIndexOf(".");
  return dot <= 0 ? "" : base.slice(dot + 1).toLowerCase();
}

/** Resolve the type metadata for a filename, falling back to plain text. */
export function fileType(filename: string): FileTypeInfo {
  const ext = extOf(filename);
  const spec = TABLE[ext];
  if (!spec) {
    return {
      ext,
      label: ext ? ext.toUpperCase() : "File",
      colorClass: GROUP_CLASS.text,
      group: "text",
      lang: "text",
      binary: false,
      icon: File,
    };
  }
  return {
    ext,
    label: spec.label,
    colorClass: GROUP_CLASS[spec.group],
    group: spec.group,
    lang: spec.lang,
    binary: spec.binary ?? false,
    icon: spec.icon,
  };
}

export interface FileIconProps {
  filename: string;
  size?: number;
  /** Tint the glyph with the type's accent colour (default true). */
  tinted?: boolean;
  className?: string;
}

// FileIcon renders the type-appropriate glyph for a filename, tinted with the
// format's accent colour so a list of mixed files is scannable at a glance.
export function FileIcon({
  filename,
  size = 16,
  tinted = true,
  className,
}: FileIconProps): React.ReactElement {
  const t = fileType(filename);
  const Icon = t.icon;
  return <Icon size={size} className={cn(tinted && t.colorClass, className)} aria-hidden />;
}
