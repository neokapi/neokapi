import React from "react";
import { useSurface, useHasDual, setSurface, type Surface } from "./store";
import styles from "./SurfaceToggle.module.css";

const OPTIONS: { value: Surface; label: string }[] = [
  { value: "cli", label: "CLI" },
  { value: "desktop", label: "Desktop" },
  { value: "both", label: "Both" },
];

// A segmented CLI / Desktop / Both control for the navbar. Renders only on pages
// that have dual-mode content (a <Cli> or <Desktop> block is mounted), and sets
// the single global `surface` preference that shows/hides those blocks site-wide.
export default function SurfaceToggle(): React.ReactElement | null {
  const surface = useSurface();
  const hasDual = useHasDual();
  if (!hasDual) return null;
  return (
    <div className={styles.toggle} role="radiogroup" aria-label="Show CLI or Kapi Desktop">
      <span className={styles.caption} aria-hidden="true">
        Show
      </span>
      {OPTIONS.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={surface === o.value}
          className={surface === o.value ? `${styles.opt} ${styles.active}` : styles.opt}
          onClick={() => setSurface(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
