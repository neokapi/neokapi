import React from "react";
import clsx from "clsx";
import { ArrowRight } from "lucide-react";
import { LANGS, REDACT_PARAMS, TERMS } from "./heroData";
import { LangCard, SlideCard } from "./parts";
import { useTimeline } from "./useTimeline";
import shell from "./shell.module.css";
import styles from "./ConceptConveyor.module.css";

// Concept C — Conveyor. The slide rides an assembly line through four stations;
// each stamps its effect as the slide passes, and the last ejects the deck into
// every reviewed language. Strong left-to-right pipeline motion.

const STATIONS = ["Redact", "Protect", "Recycle", "Translate"];
const STAMPS = ["Redacted", "Terms locked", "From memory", ""];
const CAPTIONS = [
  "Redact by policy — sensitive figures never reach a model.",
  "Feed the termbase — protected terms held verbatim, everywhere.",
  "Recycle translation memory — only pay to translate what changed.",
  "Call AI across every language — human-reviewed, written back byte-for-byte.",
];
const DURATIONS = [2500, 2700, 2500, 3800];

export default function ConceptConveyor({ onOpen }: { onOpen: () => void }): React.ReactElement {
  const { phase, setPhase, reduced, hoverBind } = useTimeline(4, DURATIONS);
  const fan = phase === 3;

  return (
    <div className={shell.card} {...hoverBind}>
      {/* Station strip — the assembly line. */}
      <div className={styles.stations} role="tablist" aria-label="kapi pipeline stations">
        {STATIONS.map((s, idx) => (
          <button
            key={s}
            type="button"
            role="tab"
            aria-selected={idx === phase}
            className={clsx(
              styles.station,
              idx === phase && styles.stationActive,
              idx < phase && styles.stationDone,
            )}
            onClick={() => setPhase(idx)}
          >
            <span className={styles.stationNo}>{idx + 1}</span>
            <span className={styles.stationName}>{s}</span>
          </button>
        ))}
        <span className={styles.track} aria-hidden="true">
          <span className={styles.trackFill} style={{ width: `${(phase / 3) * 100}%` }} />
        </span>
      </div>

      {/* The belt — the slide rides through; the last station ejects languages. */}
      <button
        type="button"
        className={styles.belt}
        onClick={onOpen}
        aria-label="Open the live demo"
      >
        <div className={styles.beltTexture} aria-hidden="true" />

        <div className={clsx(styles.carriage, fan && styles.carriageOut)}>
          <SlideCard
            redacted={REDACT_PARAMS.map((p) => p.key)}
            protectedTerms={phase >= 1 ? TERMS.map((t) => t.term) : []}
            recycled={phase >= 2}
            tint={fan ? "green" : "en"}
            noFooter
            className={styles.slide}
          />
          {!fan && STAMPS[phase] && (
            <span className={styles.stamp} key={phase} aria-hidden="true">
              {STAMPS[phase]}
            </span>
          )}
        </div>

        {/* Ejected languages roll out on the belt. */}
        <div className={clsx(styles.eject, fan && styles.ejectIn)} aria-hidden="true">
          {LANGS.map((l, idx) => (
            <LangCard
              key={l.code}
              lang={l}
              className={styles.ejectCard}
              style={reduced ? undefined : { transitionDelay: `${idx * 60}ms` }}
            />
          ))}
        </div>
      </button>

      <p className={shell.caption} key={phase}>
        <strong>{STATIONS[phase]}.</strong> {CAPTIONS[phase]}
      </p>

      <button type="button" className={shell.cta} onClick={onOpen}>
        Try it in your browser <ArrowRight size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
