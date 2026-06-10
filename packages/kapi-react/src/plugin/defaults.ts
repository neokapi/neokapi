/**
 * W3C HTML5 HTML5 default translatability rules.
 * Pure data — no AST dependencies. Shared between Babel and unplugin adapters.
 */

export const nonTranslatableElements = new Set([
  "script",
  "style",
  "code",
  "var",
  "kbd",
  "samp",
  "pre",
  "textarea",
]);

export const translatableElements = new Set([
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "p",
  "li",
  "dt",
  "dd",
  "figcaption",
  "caption",
  "summary",
  "blockquote",
  "td",
  "th",
  "label",
  "legend",
  "option",
  "optgroup",
  "button",
  "a",
  "span",
  "strong",
  "em",
  "b",
  "i",
  "u",
  "s",
  "small",
  "mark",
  "abbr",
  "cite",
  "q",
  "dfn",
  "sub",
  "sup",
  "time",
  "data",
  "ruby",
  "bdi",
  "bdo",
  "ins",
  "del",
]);

export const inlineElements = new Set([
  "a",
  "abbr",
  "b",
  "bdi",
  "bdo",
  "br",
  "cite",
  "data",
  "dfn",
  "em",
  "i",
  "kbd",
  "mark",
  "q",
  "ruby",
  "s",
  "samp",
  "small",
  "span",
  "strong",
  "sub",
  "sup",
  "time",
  "u",
  "var",
  "wbr",
  "del",
  "ins",
]);

export const containerElements = new Set([
  "div",
  "section",
  "article",
  "aside",
  "main",
  "nav",
  "header",
  "footer",
  "form",
  "fieldset",
  "details",
  "dialog",
  "figure",
  "table",
  "thead",
  "tbody",
  "tfoot",
  "tr",
  "colgroup",
  "col",
  "ul",
  "ol",
  "dl",
  "menu",
  "hgroup",
  "search",
  "output",
  "template",
]);

/**
 * Attributes extracted as translatable string literals regardless of
 * the host element. Three buckets:
 *
 *   1. HTML standard: alt, title, placeholder — user-visible text
 *      on standard elements.
 *   2. ARIA: aria-label / aria-description / aria-placeholder /
 *      aria-roledescription / aria-valuetext — always user-visible.
 *   3. React-component conventions: subtitle, description, label,
 *      heading, caption, helpText, helperText, errorMessage, hint,
 *      tooltip, emptyMessage, emptyStateText, filterPlaceholder.
 *      These are the prop names UI libraries (shadcn, mui, radix
 *      wrappers, our own `<PageHeader>` / `<EmptyState>` /
 *      `<SelectableList>` / `<FlowsWorkspace>`) use for visible
 *      text. Adding them to the default set means `<PageHeader
 *      title="Translation Memories" />` just works.
 *
 * Opt out a specific site with `translate="no"` or a rule selector.
 */
export const translatableAttributes = new Set([
  "alt",
  "title",
  "placeholder",
  "aria-label",
  "aria-description",
  "aria-placeholder",
  "aria-roledescription",
  "aria-valuetext",
  "subtitle",
  "description",
  "label",
  "heading",
  "caption",
  "helpText",
  "helperText",
  "errorMessage",
  "hint",
  "tooltip",
  "emptyMessage",
  "emptyStateText",
  "filterPlaceholder",
]);

export function getTranslatability(htmlElement: string): "yes" | "no" | "container" {
  if (nonTranslatableElements.has(htmlElement)) return "no";
  if (translatableElements.has(htmlElement)) return "yes";
  if (containerElements.has(htmlElement)) return "container";
  return "container";
}
