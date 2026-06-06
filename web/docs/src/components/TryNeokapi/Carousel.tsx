import React, { useEffect, useRef, useState } from "react";
import FormatPreview from "@neokapi/kapi-lab/FormatPreview";
import { ArrowRight, RotateCw } from "lucide-react";
import { HERO_CAROUSEL } from "./heroCarousel";
import styles from "./styles.module.css";

// The Format-carousel hero: a compact card that auto-cycles through a rendered
// slide → sheet → doc, each typewriter-revealing EN → FR with the changed words
// highlighted, then advancing. Clicking anywhere opens the full Try-Neokapi
// modal (the live-engine showcase).
//
// ZERO-WASM: this renders BAKED RenderDoc data (heroCarousel.ts) through the
// shared FormatPreview component — no engine boots on page load. The structure
// is faithful to the real extraction; the modal proves it live.
//
// prefers-reduced-motion: the auto-cycle and the EN→FR typewriter are disabled;
// a static side-by-side before/after is shown instead.

const REVEAL_MS = 1100; // EN sits before the FR reveal
const HOLD_MS = 2200; // FR sits before advancing to the next format

interface CarouselProps {
  /** Open the full modal (the card is one big button). */
  onOpen: () => void;
}

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

export default function Carousel({ onOpen }: CarouselProps): React.ReactElement {
  const reduced = usePrefersReducedMotion();
  const [index, setIndex] = useState(0);
  // "en" shows the source; "fr" reveals the translated result with highlights.
  const [phase, setPhase] = useState<"en" | "fr">("en");
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (reduced) return; // static: no auto-animation
    const schedule = () => {
      if (phase === "en") {
        timer.current = setTimeout(() => setPhase("fr"), REVEAL_MS);
      } else {
        timer.current = setTimeout(() => {
          setPhase("en");
          setIndex((i) => (i + 1) % HERO_CAROUSEL.length);
        }, HOLD_MS);
      }
    };
    schedule();
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [phase, index, reduced]);

  const item = HERO_CAROUSEL[index];
  const showingFr = phase === "fr";

  function selectFormat(i: number): void {
    if (timer.current) clearTimeout(timer.current);
    setIndex(i);
    setPhase("en");
  }

  // Reduced-motion: a calm static before/after, no auto-cycle, no cross-fade.
  if (reduced) {
    return (
      <div className={styles.carouselCard}>
        <button
          type="button"
          className={styles.carouselOpen}
          onClick={onOpen}
          aria-label="Open the interactive Try Neokapi showcase"
        >
          <div className={styles.carouselChrome}>
            <span className={styles.carouselFile}>{item.filename}</span>
            <span className={styles.carouselLangs}>
              EN <ArrowRight size={12} aria-hidden="true" /> FR
            </span>
          </div>
          <div className={styles.carouselStaticGrid}>
            <FormatPreview doc={item.source} className={styles.carouselDoc} gridHeaders={false} />
            <FormatPreview
              doc={item.target}
              before={item.source}
              className={styles.carouselDoc}
              gridHeaders={false}
              reducedMotion
            />
          </div>
        </button>
        <CarouselRail items={HERO_CAROUSEL} index={index} onSelect={selectFormat} live={false} />
        <p className={styles.carouselCaption}>Read, change, and ship content in any format.</p>
      </div>
    );
  }

  return (
    <div className={styles.carouselCard}>
      <button
        type="button"
        className={styles.carouselOpen}
        onClick={onOpen}
        aria-label="Open the interactive Try Neokapi showcase"
      >
        <div className={styles.carouselChrome}>
          <span className={styles.carouselFile}>{item.filename}</span>
          <span className={styles.carouselLangs} data-active={showingFr ? "fr" : "en"}>
            EN <ArrowRight size={12} aria-hidden="true" /> FR
          </span>
        </div>
        <div className={styles.carouselStage}>
          {/* A short, polite live-region note so screen readers hear the change. */}
          <span className={styles.srOnly} aria-live="polite">
            {item.filename}: {showingFr ? "translated to French" : "English source"}
          </span>
          <div key={`${item.id}-${phase}`} className={styles.carouselReveal}>
            <FormatPreview
              doc={showingFr ? item.target : item.source}
              before={showingFr ? item.source : undefined}
              transition={showingFr ? "typewriter" : "none"}
              typewriter="word"
              className={styles.carouselDoc}
              gridHeaders={false}
            />
          </div>
        </div>
      </button>
      <CarouselRail items={HERO_CAROUSEL} index={index} onSelect={selectFormat} live />
      <p className={styles.carouselCaption}>Read, change, and ship content in any format.</p>
    </div>
  );
}

function CarouselRail({
  items,
  index,
  onSelect,
  live,
}: {
  items: typeof HERO_CAROUSEL;
  index: number;
  onSelect: (i: number) => void;
  live: boolean;
}): React.ReactElement {
  return (
    <div className={styles.carouselRail}>
      <div className={styles.carouselDots} role="tablist" aria-label="Pick a format to preview">
        {items.map((it, i) => (
          <button
            key={it.id}
            type="button"
            role="tab"
            aria-selected={i === index}
            aria-label={it.label}
            className={`${styles.carouselDot} ${i === index ? styles.carouselDotActive : ""}`}
            onClick={() => onSelect(i)}
          >
            <span className={styles.carouselDotMark} aria-hidden="true" />
            <span className={styles.carouselDotLabel}>{it.label}</span>
          </button>
        ))}
      </div>
      {live && (
        <span className={styles.carouselCycle} aria-hidden="true">
          EN <ArrowRight size={11} /> FR <RotateCw size={11} className={styles.carouselSpin} />
        </span>
      )}
    </div>
  );
}
