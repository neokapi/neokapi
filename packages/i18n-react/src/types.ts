export type PluginOptions = {
  /** 'inline' = build-time (default when locale set), 'runtime' = OTA dynamic loading */
  mode?: "inline" | "runtime";

  /** Target locale for inline mode (e.g., "de", "ja", "qps"). */
  locale?: string;

  /**
   * Fallback locale chain. When a translation is missing in the primary locale,
   * try these locales in order before falling back to source text.
   * E.g., ['de', 'en'] — try de-AT first, then de, then en, then source text.
   */
  fallbackLocales?: string[];

  /** Directory containing translation files ({locale}.json). Default: "./translations" */
  translationsDir?: string;

  /** Maps custom React components to their rendered HTML element. */
  componentMap?: Record<string, string>;

  /**
   * Override translatability rules for specific elements, classes, or attributes.
   * Selectors: element name, .className, [attribute], [attribute=value]
   */
  rules?: Array<{
    selector: string;
    translate?: boolean;
    locNote?: string;
  }>;

  /** Path to community-maintained i18n manifests for third-party libraries. */
  communityManifestDir?: string;

  /**
   * Project root for resolving library i18n manifests. Used by the
   * auto-detection pipeline when falling back to parsing `.d.ts`
   * files for `RefAttributes<HTMLXxxElement>` hints. Defaults to
   * `process.cwd()`.
   */
  projectRoot?: string;

  /** Warn about unmapped components with translatable text. Default: true in dev. */
  warnUnmapped?: boolean;

  /**
   * Override how translatability warnings (auto-promoted containers,
   * unmapped components) are surfaced. Defaults to `console.warn`.
   * Useful for tests or to integrate with a project's logger.
   */
  onWarning?: (message: string) => void;

  /**
   * How to handle missing translations during inline builds.
   *   'warn'  — log a warning and fall back to source text (default)
   *   'error' — throw a build error
   *   false   — silently fall back to source text
   */
  strict?: "warn" | "error" | false;

  /**
   * In-context review mode: stamp every extracted element with
   * `data-kapi-id` (block hash), `data-kapi-loc` (file:line), and
   * `data-kapi-attr` (attribute-block hashes) so the review overlay
   * (`@neokapi/i18n-react/review`) can map DOM → block. In the Vite
   * dev server this also mounts the review middleware at
   * `/__kapi/review` (payloads from the KBF tree, write-back, SSE)
   * and auto-injects the overlay into index.html. Enable explicitly
   * or via the KAPI_REVIEW=1 environment variable. Never enable for
   * production builds you ship.
   */
  review?: boolean;

  /**
   * KBF tree the review middleware serves and writes back to.
   * Default: "i18n" (the extract default).
   */
  reviewKbfDir?: string;

  /**
   * Promote extraction-time warnings (e.g. `unknown-component`) to
   * build errors. Orthogonal to `strict` above: `strict` is about
   * translation completeness at inline time, `warningsAsErrors` is
   * about authoring-time issues the walker records.
   *
   * The `neokapi-i18n extract --strict` CLI flag sets this.
   * Defaults to `false`; honours `process.env.CI` only when the caller
   * explicitly opts in (we don't force-promote warnings just because
   * CI=true — too easy to break unrelated builds).
   */
  warningsAsErrors?: boolean;
};

/** Unit Separator — delimits context from translator note in hash computation. */
export const CONTEXT_SEPARATOR = "\x1F";

/**
 * Hash descriptor channel for document-head translatables — the
 * `<title>`, `<meta name="description">`, and Open Graph / Twitter
 * card strings set through the `@neokapi/i18n-react/head` hooks.
 *
 * Head strings carry no DOM text node, so the build transform never
 * rewrites them; instead the head hook self-hashes its source with
 * this descriptor at runtime and the extractor emits the matching
 * block. Using a dedicated channel (not the `t()` channel) keeps a
 * page title distinct from an identically-worded JS label, while
 * every head kind sharing one descriptor lets `<title>` and
 * `og:title` reuse a single translation when their source matches.
 */
export const HEAD_DESCRIPTOR = "head";
