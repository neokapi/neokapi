import { useState } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import "./ThemedVideo.css";

interface ThemedVideoProps {
  sources: {
    light: string;
    dark: string;
  };
  maxWidth?: string;
}

// Localized asset name for a docs locale: the harness publishes locale variants
// with the locale inserted before the theme suffix (`foo-dark.webm` →
// `foo-nb-dark.webm`, poster alike); theme-agnostic files get a plain suffix
// (`foo.webm` → `foo-nb.webm`).
function localizeAsset(p: string, locale: string): string {
  if (/-(light|dark)\.webm$/.test(p)) return p.replace(/-(light|dark)\.webm$/, `-${locale}-$1.webm`);
  return p.replace(/\.webm$/, `-${locale}.webm`);
}

// A theme-aware <video>. Both variants are emitted into the DOM and toggled
// with CSS keyed off Docusaurus's `data-theme` attribute, rather than reading
// the color mode through useColorMode(). That hook lives in
// @docusaurus/theme-common, which resolves to a *different* module instance for
// workspace-package consumers like this one — so calling it threw
// "ReactContextError: Hook is called outside the <ColorModeProvider>" and blanked
// the whole page. CSS toggling has no React-context dependency, renders during
// SSG, and avoids a flash on theme switch.
//
// Source paths run through useBaseUrl so they respect the site's baseUrl
// (e.g. /web/neokapi/) instead of resolving against the domain root, which
// is why the recorded scenes 404'd despite being deployed.
//
// When both variants are the same file (theme-agnostic terminal recordings) a
// single element is emitted to avoid a duplicate download.
//
// A poster is derived from each source (`foo.webm` → `foo.jpg`) so the player
// shows a content frame instead of a flat, blank first frame — which is
// invisible once the video is theme-matched to the page background. A missing
// poster simply falls back to the first frame.
//
// Localized docs (Docusaurus i18n locale ≠ en) prefer the `-<locale>` asset
// variant; when that file hasn't been published yet, the <source> error
// handler swaps back to the English asset (the `key` forces the <video> to
// re-evaluate its source). The English locale takes the original path
// untouched — exactly the previous behavior.
export default function ThemedVideo({ sources, maxWidth = "800px" }: ThemedVideoProps) {
  const { i18n } = useDocusaurusContext();
  const locale = i18n?.currentLocale ?? "en";
  const [useLocalized, setUseLocalized] = useState(locale !== "en");
  const pick = (p: string) => (useLocalized ? localizeAsset(p, locale) : p);
  const light = useBaseUrl(pick(sources.light));
  const dark = useBaseUrl(pick(sources.dark));
  const posterLight = light.replace(/\.webm$/, ".jpg");
  const posterDark = dark.replace(/\.webm$/, ".jpg");
  const onSourceError = useLocalized ? () => setUseLocalized(false) : undefined;

  if (light === dark) {
    return (
      <video key={light} controls width="100%" style={{ maxWidth }} poster={posterLight} preload="metadata">
        <source src={light} type="video/webm" onError={onSourceError} />
        Your browser does not support the video tag.
      </video>
    );
  }

  return (
    <>
      <video
        key={light}
        className="themed-video themed-video--light"
        controls
        width="100%"
        style={{ maxWidth }}
        poster={posterLight}
        preload="metadata"
      >
        <source src={light} type="video/webm" onError={onSourceError} />
        Your browser does not support the video tag.
      </video>
      <video
        key={dark}
        className="themed-video themed-video--dark"
        controls
        width="100%"
        style={{ maxWidth }}
        poster={posterDark}
        preload="metadata"
      >
        <source src={dark} type="video/webm" onError={onSourceError} />
        Your browser does not support the video tag.
      </video>
    </>
  );
}
