// Ambient declarations so the package typechecks on its own. At runtime these
// modules are supplied by the consuming Docusaurus site (which compiles the TSX
// and resolves the CSS side-effect imports through its webpack config); this
// file only satisfies a standalone `tsc --noEmit`.

declare module "*.css";

declare module "@docusaurus/useBaseUrl" {
  export default function useBaseUrl(url: string): string;
}

declare module "@docusaurus/useDocusaurusContext" {
  interface SiteConfig {
    customFields?: Record<string, unknown>;
    [key: string]: unknown;
  }
  export default function useDocusaurusContext(): { siteConfig: SiteConfig };
}
