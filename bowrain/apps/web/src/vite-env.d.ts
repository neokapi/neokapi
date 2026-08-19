/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_POSTHOG_KEY?: string;
  readonly VITE_POSTHOG_HOST?: string;
  /**
   * The origin in-context review frames previews through — the shim that stands
   * between this app and a customer's own preview host.
   *
   * The twin of this deployment's content policy: the policy names this origin
   * in frame-src (bowrain-infra, modules/spa-site's csp_frame_src, fed by
   * modules/embed-origin), and the two must agree or every frame is refused.
   * Unset in development, where in-context reading is not offered at all rather
   * than offered and blocked.
   */
  readonly VITE_EMBED_ORIGIN?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}

declare module "@fontsource-variable/dm-sans";
declare module "@fontsource/dm-mono/*";
