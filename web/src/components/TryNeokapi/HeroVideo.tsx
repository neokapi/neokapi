import React, { useEffect, useState } from "react";
import useBaseUrl from "@docusaurus/useBaseUrl";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import { readCdnConfig, cdnEnabled, cdnHref } from "@neokapi/docs-shared";
import { ArrowRight } from "lucide-react";
import styles from "./HeroVideo.module.css";

// The landing centerpiece: a seamlessly-looping, theme-matched cinematic loop
// (rendered with Remotion — see harness/src/remotion/compositions/ContentLoop.tsx)
// plus the CTA into the live in-browser engine. A borderless autoplay/loop/muted
// video, not the controls-bearing ThemedVideo, so it reads as a motion piece
// rather than an embedded clip — but it reuses the same light/dark CSS toggle
// convention (keyed off Docusaurus's data-theme) and the same CDN routing.
//
// Zero-wasm on load: a <video> pulls no engine; the modal (which boots the wasm
// runtime) only mounts when the reader clicks the CTA.
//
// prefers-reduced-motion: the autoplaying video is suppressed entirely and only
// the (paused) poster frame is shown — no motion, no decode.

interface HeroVideoProps {
  /** Open the full live-engine modal. */
  onOpen: () => void;
}

const SOURCES = {
  light: "/video/content-loop-light.webm",
  dark: "/video/content-loop-dark.webm",
};

function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    setReduced(mq.matches);
    const on = (e: MediaQueryListEvent) => setReduced(e.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  }, []);
  return reduced;
}

export default function HeroVideo({ onOpen }: HeroVideoProps): React.ReactElement {
  const reduced = usePrefersReducedMotion();
  const { siteConfig } = useDocusaurusContext();

  // Resolve each source against the CDN when an origin is configured (production),
  // else against the site base URL (local dev + PR preview, where the assets are
  // committed under web/static/video). Both useBaseUrl calls run unconditionally
  // (hooks rule); the CDN form takes precedence when enabled.
  const cdn = readCdnConfig(siteConfig);
  const onCdn = cdnEnabled(cdn);
  const lightLocal = useBaseUrl(SOURCES.light);
  const darkLocal = useBaseUrl(SOURCES.dark);
  const light = onCdn ? cdnHref(cdn, SOURCES.light) : lightLocal;
  const dark = onCdn ? cdnHref(cdn, SOURCES.dark) : darkLocal;
  const posterLight = light.replace(/\.webm$/, ".jpg");
  const posterDark = dark.replace(/\.webm$/, ".jpg");

  // Shared props for the borderless looping hero video.
  const videoProps: React.VideoHTMLAttributes<HTMLVideoElement> = {
    autoPlay: true,
    muted: true,
    loop: true,
    playsInline: true,
    preload: "auto",
    "aria-hidden": true,
    tabIndex: -1,
  };

  return (
    <div className={styles.hero}>
      <button
        type="button"
        className={styles.stage}
        onClick={onOpen}
        aria-label="Open the interactive kapi engine in your browser"
      >
        <span className={styles.glow} aria-hidden="true" />
        {reduced ? (
          // Reduced motion: poster only (paused), theme-toggled like the videos.
          <>
            <img
              src={posterLight}
              alt=""
              className={`${styles.media} ${styles.mediaLight}`}
              aria-hidden="true"
            />
            <img
              src={posterDark}
              alt=""
              className={`${styles.media} ${styles.mediaDark}`}
              aria-hidden="true"
            />
          </>
        ) : (
          <>
            <video
              {...videoProps}
              key={light}
              className={`${styles.media} ${styles.mediaLight}`}
              poster={posterLight}
            >
              <source src={light} type="video/webm" />
            </video>
            <video
              {...videoProps}
              key={dark}
              className={`${styles.media} ${styles.mediaDark}`}
              poster={posterDark}
            >
              <source src={dark} type="video/webm" />
            </video>
          </>
        )}
      </button>

      <button type="button" className={styles.cta} onClick={onOpen}>
        Try kapi in your browser <ArrowRight size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
