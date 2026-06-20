// CDN asset routing — shared by both docs sites.
//
// The large, immutable docs assets (the kapi wasm engine, the ONNX vision
// models, and the walkthrough videos) can be offloaded to an external CDN
// (Cloudflare R2 served at e.g. https://cdn.bowrain.cloud) so the GitHub Pages
// artifact stays small and deploys fast. The site reads the CDN origin from a
// build-time customField (`cdnBaseUrl`, populated from $DOCS_CDN_URL). When it
// is empty — the default, and the local-dev case — every asset stays
// same-origin and nothing changes.
//
// One bucket backs both sites, so each scopes its objects under a per-site
// prefix (`cdnSitePrefix`: "kapi" | "bowrain"). The per-build wasm is additionally
// versioned (`cdnWasmVersion`, a git sha) so it can be served immutably without
// a new deploy serving a stale binary.

export interface CdnConfig {
  /** CDN origin without a trailing slash, e.g. "https://cdn.bowrain.cloud". Empty = disabled. */
  base: string;
  /** Per-site object scope within the shared bucket, e.g. "kapi" or "bowrain". */
  sitePrefix: string;
  /** Cache-busting path segment for per-build assets (the wasm engine), e.g. a git sha. */
  version: string;
  /**
   * Version segment for the Vision Lab ONNX model set, e.g. "v1" — pinned in
   * web/models.version and committed, so bumping it is a reviewable diff and the
   * PR's preview loads /kapi/models/vision/<modelsVersion>/. Unlike `version`
   * (per-build wasm), the model set changes rarely and independently of the sha.
   */
  modelsVersion: string;
}

/** The subset of Docusaurus's siteConfig this module reads. */
interface SiteConfigLike {
  customFields?: { [key: string]: unknown };
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

/** Read the CDN configuration baked into the site's customFields. */
export function readCdnConfig(siteConfig: SiteConfigLike): CdnConfig {
  const cf = siteConfig.customFields ?? {};
  return {
    base: str(cf.cdnBaseUrl).replace(/\/+$/, ""),
    sitePrefix: str(cf.cdnSitePrefix),
    version: str(cf.cdnWasmVersion) || "dev",
    modelsVersion: str(cf.cdnModelsVersion) || "v1",
  };
}

/** Whether assets should be served from the CDN (an origin is configured). */
export function cdnEnabled(cfg: CdnConfig): boolean {
  return cfg.base !== "";
}

/**
 * Join the CDN origin, the per-site scope, and a site-relative asset path into
 * an absolute URL, e.g. cdnHref({base:"https://cdn.x", sitePrefix:"kapi"}, "/video/a.webm")
 * → "https://cdn.x/kapi/video/a.webm". The caller is responsible for only
 * calling this when {@link cdnEnabled} is true.
 */
export function cdnHref(cfg: CdnConfig, relPath: string): string {
  const rel = relPath.startsWith("/") ? relPath : `/${relPath}`;
  const scope = cfg.sitePrefix ? `/${cfg.sitePrefix}` : "";
  return `${cfg.base}${scope}${rel}`;
}
