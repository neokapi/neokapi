import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "./cdn";
import "./ThemedImage.css";

interface ThemedImageProps {
  alt: string;
  sources: {
    light: string;
    dark: string;
  };
  className?: string;
}

// A theme-aware <img>, mirroring ThemedVideo. Both variants are emitted into the
// DOM and toggled with CSS keyed off Docusaurus's `data-theme` attribute, rather
// than reading the color mode through useColorMode() — that hook lives in
// @docusaurus/theme-common, which resolves to a *different* module instance for
// workspace-package consumers like this one, throwing a ReactContextError that
// blanks the page (the same reason ThemedVideo toggles via CSS).
//
// Crucially, source paths run through useBaseUrl so they respect the site's
// baseUrl (e.g. /web/bowrain/docs/) instead of resolving against the domain
// root. The built-in @theme/ThemedImage does NOT do this, so raw "/img/…" paths
// 404 in the browser despite the assets being deployed under the baseUrl.
//
// When both variants are the same file a single <img> is emitted.
//
// Source paths resolve against the site baseUrl, or — when a CDN origin is
// configured (cdnBaseUrl customField, from $DOCS_CDN_URL) — against the CDN,
// exactly as ThemedVideo does. This keeps large, release-only screenshots out
// of the GitHub Pages artifact and the PR-preview bundle: they live once on R2
// (published via `make publish-cdn-images` / `publish-cdn-bowrain-images`) and
// are referenced by URL. useBaseUrl runs unconditionally (hooks rule); the CDN
// form simply takes precedence.
export default function ThemedImage({ alt, sources, className }: ThemedImageProps) {
  const { siteConfig } = useDocusaurusContext();
  const cdn = readCdnConfig(siteConfig);
  const onCdn = cdnEnabled(cdn);
  const lightLocal = useBaseUrl(sources.light);
  const darkLocal = useBaseUrl(sources.dark);
  const light = onCdn ? cdnHref(cdn, sources.light) : lightLocal;
  const dark = onCdn ? cdnHref(cdn, sources.dark) : darkLocal;
  const cls = className ? ` ${className}` : "";

  if (light === dark) {
    return <img className={`themed-img${cls}`} src={light} alt={alt} loading="lazy" />;
  }

  return (
    <>
      <img className={`themed-img themed-img--light${cls}`} src={light} alt={alt} loading="lazy" />
      <img className={`themed-img themed-img--dark${cls}`} src={dark} alt={alt} loading="lazy" />
    </>
  );
}
