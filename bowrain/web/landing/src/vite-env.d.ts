/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_POSTHOG_KEY?: string;
  readonly VITE_POSTHOG_HOST?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

declare module "@fontsource-variable/fraunces";
declare module "@fontsource-variable/inter";
declare module "@fontsource-variable/jetbrains-mono";

// Injected by vite.config.ts (see its `define` block). The config is the only
// place that knows the deploy shape, so the locale menu and the cross-site docs
// URL are computed there rather than derived from BASE_URL in the app.
declare const __BUILD_STAMP__: string;
declare const __DOCS_BASE__: string;
declare const __LOCALE__: string;
declare const __LOCALES__: { code: string; label: string; href: string }[];
