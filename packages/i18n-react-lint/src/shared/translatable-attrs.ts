/**
 * Attribute names that neokapi-i18n extracts. Keep in sync with
 * packages/i18n-react's defaults (`translatableAttributes`) —
 * duplicated rather than imported so this package stays usable
 * without installing the full neokapi-i18n transform, and guarded by
 * tests/attr-sync.test.ts which compares against the canonical set.
 *
 * Note the extractor scopes the React-convention bucket to PascalCase
 * components; the lint vocabulary stays the union (a flagged concat
 * in `label=` on an html element is still worth a look).
 */
export const TRANSLATABLE_ATTRS: ReadonlySet<string> = new Set([
  // HTML
  "alt",
  "title",
  "placeholder",
  // ARIA
  "aria-label",
  "aria-description",
  "aria-placeholder",
  "aria-roledescription",
  "aria-valuetext",
  // React conventions
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

/**
 * Object-literal keys we treat as "likely user-facing strings" when
 * checking data-array patterns like `const ITEMS = [{ label: 'Foo' }]`.
 * Intentionally narrower than TRANSLATABLE_ATTRS — must be a strong
 * signal to keep false positives low.
 *
 * Explicitly excluded: `name`, `description`, `text`, `message`. Those
 * overwhelmingly name backend / runtime data in real React apps
 * (plugin.name, error.message, schema.description, file.name, …) that
 * isn't authoring-time translatable, so flagging them fires mostly
 * false positives. The remaining set skews strongly toward hardcoded
 * UI copy.
 */
export const LIKELY_LABEL_KEYS: ReadonlySet<string> = new Set([
  "label",
  "title",
  "heading",
  "caption",
  "subtitle",
  "tooltip",
  "placeholder",
  "summary",
]);
