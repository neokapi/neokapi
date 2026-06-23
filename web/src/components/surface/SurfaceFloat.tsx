import React, { useEffect, useState } from "react";
import { Filter, Terminal, AppWindow } from "lucide-react";
import { useEnabled, useHasDual, toggleSurface } from "./store";
import styles from "./SurfaceFloat.module.css";

// A floating control that sits in the top-right (the right-TOC gutter), fading in
// once the reader scrolls into the content and out at the top. Shown only on
// pages with dual-mode content. A filter icon labels two independent toggles —
// CLI and Kapi Desktop — each on/off; together they set the global surface
// preference that shows/hides <Cli> / <Desktop> blocks.
export default function SurfaceFloat(): React.ReactElement | null {
  const enabled = useEnabled();
  const hasDual = useHasDual();
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 64);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  if (!hasDual) return null;

  return (
    <div
      className={`${styles.float} ${scrolled ? styles.shown : styles.hidden}`}
      role="group"
      aria-label="Show CLI or Kapi Desktop content"
    >
      <Filter className={styles.filterIcon} size={15} aria-hidden="true" />
      <button
        type="button"
        aria-pressed={enabled.cli}
        aria-label="Show CLI"
        title="CLI"
        className={enabled.cli ? `${styles.opt} ${styles.on}` : styles.opt}
        onClick={() => toggleSurface("cli")}
      >
        <Terminal size={16} aria-hidden="true" />
      </button>
      <button
        type="button"
        aria-pressed={enabled.desktop}
        aria-label="Show Kapi Desktop"
        title="Kapi Desktop"
        className={enabled.desktop ? `${styles.opt} ${styles.on}` : styles.opt}
        onClick={() => toggleSurface("desktop")}
      >
        <AppWindow size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
