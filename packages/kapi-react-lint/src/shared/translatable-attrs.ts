/**
 * Attribute names that kapi-react extracts from any JSX element
 * (mapped or not). Keep in sync with packages/kapi-react's defaults.
 * Duplicated rather than imported so this package stays usable without
 * installing the full kapi-react transform.
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
]);

/**
 * Object-literal keys we treat as "likely user-facing strings" when
 * checking data-array patterns like `const ITEMS = [{ label: 'Foo' }]`.
 * Intentionally narrower than TRANSLATABLE_ATTRS — must be a strong
 * signal to keep false positives low.
 */
export const LIKELY_LABEL_KEYS: ReadonlySet<string> = new Set([
  "label",
  "title",
  "heading",
  "caption",
  "description",
  "subtitle",
  "tooltip",
  "placeholder",
  "text",
  "name",
]);
