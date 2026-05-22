import useBaseUrl from "@docusaurus/useBaseUrl";
import "./ThemedVideo.css";

interface ThemedVideoProps {
  sources: {
    light: string;
    dark: string;
  };
  maxWidth?: string;
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
// (e.g. /web/neokapi/docs/) instead of resolving against the domain root, which
// is why the recorded scenes 404'd despite being deployed.
//
// When both variants are the same file (theme-agnostic terminal recordings) a
// single element is emitted to avoid a duplicate download.
export default function ThemedVideo({ sources, maxWidth = "800px" }: ThemedVideoProps) {
  const light = useBaseUrl(sources.light);
  const dark = useBaseUrl(sources.dark);

  if (light === dark) {
    return (
      <video controls width="100%" style={{ maxWidth }}>
        <source src={light} type="video/webm" />
        Your browser does not support the video tag.
      </video>
    );
  }

  return (
    <>
      <video
        className="themed-video themed-video--light"
        controls
        width="100%"
        style={{ maxWidth }}
        preload="metadata"
      >
        <source src={light} type="video/webm" />
        Your browser does not support the video tag.
      </video>
      <video
        className="themed-video themed-video--dark"
        controls
        width="100%"
        style={{ maxWidth }}
        preload="metadata"
      >
        <source src={dark} type="video/webm" />
        Your browser does not support the video tag.
      </video>
    </>
  );
}
