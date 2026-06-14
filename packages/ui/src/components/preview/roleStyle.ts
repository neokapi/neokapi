// roleStyle — map a block's normalized SemanticRole (the WS1 structural layer,
// core/model/structure.go) to a short label + accent, for the Structure and
// Layout views. The role vocabulary is open; an unknown role degrades to a
// neutral accent with a humanized label, never an error.

export interface RoleStyle {
  /** Short human label for a chip/badge. */
  label: string;
  /** Tailwind accent classes (bg + text, light/dark), matching overlayHighlight. */
  className: string;
}

// Keyed on the model.Role* constants (structure.go). Heading/title are the
// document spine (indigo); list/table content shares a family; captions,
// footnotes and furniture (page headers/footers) read as secondary.
const ROLE_STYLES: Record<string, RoleStyle> = {
  title: { label: "Title", className: "bg-indigo-500/20 text-indigo-700 dark:text-indigo-300" },
  heading: { label: "Heading", className: "bg-indigo-500/20 text-indigo-700 dark:text-indigo-300" },
  section: { label: "Section", className: "bg-indigo-500/15 text-indigo-700 dark:text-indigo-300" },
  paragraph: {
    label: "Paragraph",
    className: "bg-slate-400/15 text-slate-700 dark:text-slate-300",
  },
  "list-item": {
    label: "List item",
    className: "bg-emerald-500/20 text-emerald-700 dark:text-emerald-300",
  },
  list: { label: "List", className: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300" },
  "table-cell": {
    label: "Cell",
    className: "bg-cyan-500/20 text-cyan-700 dark:text-cyan-300",
  },
  "table-header": {
    label: "Header cell",
    className: "bg-cyan-500/30 text-cyan-800 dark:text-cyan-200",
  },
  table: { label: "Table", className: "bg-cyan-500/15 text-cyan-700 dark:text-cyan-300" },
  caption: { label: "Caption", className: "bg-amber-500/20 text-amber-700 dark:text-amber-300" },
  footnote: { label: "Footnote", className: "bg-amber-500/15 text-amber-700 dark:text-amber-300" },
  code: { label: "Code", className: "bg-fuchsia-500/20 text-fuchsia-700 dark:text-fuchsia-300" },
  formula: {
    label: "Formula",
    className: "bg-fuchsia-500/15 text-fuchsia-700 dark:text-fuchsia-300",
  },
  picture: { label: "Picture", className: "bg-rose-500/15 text-rose-700 dark:text-rose-300" },
  "page-header": {
    label: "Page header",
    className: "bg-zinc-500/15 text-zinc-600 dark:text-zinc-400",
  },
  "page-footer": {
    label: "Page footer",
    className: "bg-zinc-500/15 text-zinc-600 dark:text-zinc-400",
  },
  "form-field": { label: "Field", className: "bg-teal-500/20 text-teal-700 dark:text-teal-300" },
};

const FALLBACK: RoleStyle = {
  label: "Block",
  className: "bg-muted text-muted-foreground",
};

/** Humanize an unknown role id ("page-header" → "Page header") for the label. */
function humanize(role: string): string {
  const s = role.replace(/[-_]/g, " ").trim();
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : "Block";
}

/**
 * roleStyle resolves a normalized role to its label + accent. A heading carries
 * its level into the label ("Heading 2"). An unknown role degrades to a
 * humanized label with a neutral accent.
 */
export function roleStyle(role: string | undefined, level?: number): RoleStyle {
  if (!role) return FALLBACK;
  const base = ROLE_STYLES[role] ?? { ...FALLBACK, label: humanize(role) };
  if ((role === "heading" || role === "title") && level && level > 0) {
    return { ...base, label: `${base.label} ${level}` };
  }
  return base;
}
