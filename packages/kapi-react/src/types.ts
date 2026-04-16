export type PluginOptions = {
  /** 'inline' = build-time (default when locale set), 'runtime' = OTA dynamic loading */
  mode?: 'inline' | 'runtime';

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

  /** Warn about unmapped components with translatable text. Default: true in dev. */
  warnUnmapped?: boolean;

  /**
   * How to handle missing translations during inline builds.
   *   'warn'  — log a warning and fall back to source text (default)
   *   'error' — throw a build error
   *   false   — silently fall back to source text
   */
  strict?: 'warn' | 'error' | false;
};

/** Unit Separator — delimits context from translator note in hash computation. */
export const CONTEXT_SEPARATOR = '\x1F';
