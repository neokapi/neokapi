import { useId, useState } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "./cdn";
import OutdatedNotice, { type OutdatedKind } from "./OutdatedNotice";
import "./ThemedVideo.css";

interface ThemedVideoProps {
  sources: {
    light: string;
    dark: string;
  };
  maxWidth?: string;
  /**
   * Marks the recording as no longer matching the current release, rendering a
   * banner above the player while the player keeps working. Opt-in per embed:
   * the page decides, because only the page knows which release its prose
   * describes. `"interface"` when the recorded screens no longer exist;
   * `"wording"` when only the narration's terminology is retired.
   */
  outdated?: OutdatedKind;
  /** An optional sentence naming the specific drift, appended to the banner. */
  outdatedNote?: string;
}

// Localized asset name for a docs locale: the harness publishes locale variants
// with the locale inserted before the theme suffix (`foo-dark.webm` →
// `foo-nb-dark.webm`, poster alike); theme-agnostic files get a plain suffix
// (`foo.webm` → `foo-nb.webm`).
function localizeAsset(p: string, locale: string): string {
  if (/-(light|dark)\.webm$/.test(p))
    return p.replace(/-(light|dark)\.webm$/, `-${locale}-$1.webm`);
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
// (e.g. a PR preview's /web/prs/<N>/neokapi/docs/) instead of resolving
// against the domain root, which is why the recorded scenes 404'd despite
// being deployed.
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
export default function ThemedVideo({
  sources,
  maxWidth = "800px",
  outdated,
  outdatedNote,
}: ThemedVideoProps) {
  const { i18n, siteConfig } = useDocusaurusContext();
  const locale = i18n?.currentLocale ?? "en";
  // Ties each <video> to the banner text via aria-describedby, so the reason
  // the recording is stale is announced with the player and not only as loose
  // prose that precedes it.
  const noticeId = `${useId()}-outdated`;
  const describedBy = outdated ? noticeId : undefined;
  const notice = outdated ? (
    <div className="themed-video__notice" style={{ maxWidth }}>
      <OutdatedNotice kind={outdated} note={outdatedNote} textId={noticeId} />
    </div>
  ) : null;
  const [useLocalized, setUseLocalized] = useState(locale !== "en");
  const pick = (p: string) => (useLocalized ? localizeAsset(p, locale) : p);
  // Resolve each source against the site base URL, or — when a CDN origin is
  // configured (cdnBaseUrl customField) — against the CDN. Both useBaseUrl calls
  // run unconditionally (hooks rule); the CDN form simply takes precedence.
  const cdn = readCdnConfig(siteConfig);
  const onCdn = cdnEnabled(cdn);
  const lightLocal = useBaseUrl(pick(sources.light));
  const darkLocal = useBaseUrl(pick(sources.dark));
  const light = onCdn ? cdnHref(cdn, pick(sources.light)) : lightLocal;
  const dark = onCdn ? cdnHref(cdn, pick(sources.dark)) : darkLocal;
  const posterLight = light.replace(/\.webm$/, ".jpg");
  const posterDark = dark.replace(/\.webm$/, ".jpg");
  const onSourceError = useLocalized ? () => setUseLocalized(false) : undefined;

  if (light === dark) {
    return (
      <>
        {notice}
        <video
          key={light}
          controls
          width="100%"
          style={{ maxWidth }}
          poster={posterLight}
          preload="metadata"
          aria-describedby={describedBy}
        >
          <source src={light} type="video/webm" onError={onSourceError} />
          Your browser does not support the video tag.
        </video>
      </>
    );
  }

  return (
    <>
      {notice}
      <video
        key={light}
        className="themed-video themed-video--light"
        controls
        width="100%"
        style={{ maxWidth }}
        poster={posterLight}
        preload="metadata"
        aria-describedby={describedBy}
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
        aria-describedby={describedBy}
      >
        <source src={dark} type="video/webm" onError={onSourceError} />
        Your browser does not support the video tag.
      </video>
    </>
  );
}
