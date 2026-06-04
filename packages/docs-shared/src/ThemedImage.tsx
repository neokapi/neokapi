import useBaseUrl from "@docusaurus/useBaseUrl";
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
export default function ThemedImage({ alt, sources, className }: ThemedImageProps) {
  const light = useBaseUrl(sources.light);
  const dark = useBaseUrl(sources.dark);
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
