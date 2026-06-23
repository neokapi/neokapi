import React from "react";
import clsx from "clsx";
import { ArrowRight, ShieldCheck } from "lucide-react";
import { LANGS, MODELS, REDACT_PARAMS, TERMS } from "./heroData";
import { LangCard, ModelBadge, ParamTag, SlideCard, TermChip } from "./parts";
import { useTimeline } from "./useTimeline";
import shell from "./shell.module.css";
import styles from "./ConceptSpotlight.module.css";

// Concept A — Spotlight. The slide is the centred star; each mechanism animates
// ONTO it on a four-beat timeline:
//   0 Redact   — policy params drop in (top-left) and the figures wipe to bars.
//   1 Protect  — term chips fly in from the right and lock onto the slide.
//   2 Recycle  — a translation-memory card slides in and recovers the claim line.
//   3 Translate— model badges rise and the slide fans into reviewed languages.
// The slide never leaves focus; the effects choreograph around it.

const PHASES = ["Redact", "Protect terms", "Recycle", "Translate"];
const CAPTIONS = [
  "Redact by policy — sensitive figures never reach a model.",
  "Feed the termbase — protected terms held verbatim, everywhere.",
  "Recycle translation memory — only pay to translate what changed.",
  "Call AI across every language — human-reviewed, written back byte-for-byte.",
];
const DURATIONS = [2600, 2800, 2600, 3800];

export default function ConceptSpotlight({ onOpen }: { onOpen: () => void }): React.ReactElement {
  const { phase, setPhase, reduced, hoverBind } = useTimeline(4, DURATIONS);

  const redacted = phase >= 0 ? REDACT_PARAMS.map((p) => p.key) : [];
  const locks = phase >= 1 ? TERMS.map((t) => t.term) : [];
  const fan = phase === 3;

  return (
    <div className={shell.card} {...hoverBind}>
      <div className={shell.rail} role="tablist" aria-label="kapi pipeline stages">
        {PHASES.map((p, idx) => (
          <button
            key={p}
            type="button"
            role="tab"
            aria-selected={idx === phase}
            aria-label={p}
            className={clsx(shell.railSeg, idx === phase && shell.railSegActive)}
            onClick={() => setPhase(idx)}
          >
            <span className={shell.railTrack}>
              <span className={shell.railFill} data-run={idx === phase && !reduced} />
            </span>
            <span className={shell.railLabel}>{p}</span>
          </button>
        ))}
      </div>

      <button
        type="button"
        className={shell.stageBtn}
        onClick={onOpen}
        aria-label="Open the live demo"
      >
        <div className={clsx(styles.stage, fan && styles.stageFan)}>
          {/* Satellites: policy params (left). */}
          <div className={clsx(styles.params, phase === 0 && styles.paramsIn)} aria-hidden="true">
            <span className={styles.satLabel}>
              <ShieldCheck size={12} strokeWidth={2.5} /> redaction policy
            </span>
            {REDACT_PARAMS.map((p, idx) => (
              <ParamTag key={p.key} active style={{ transitionDelay: `${idx * 90}ms` }}>
                redact: {p.label}
              </ParamTag>
            ))}
          </div>

          {/* Satellites: protected terms (right). */}
          <div className={clsx(styles.terms, phase === 1 && styles.termsIn)} aria-hidden="true">
            <span className={styles.satLabel}>termbase</span>
            {TERMS.map((t, idx) => (
              <TermChip
                key={t.term}
                term={t.term}
                domain={t.domain}
                locked
                style={{ transitionDelay: `${idx * 90}ms` }}
              />
            ))}
          </div>

          {/* Satellite: translation memory (left). */}
          <div className={clsx(styles.memory, phase === 2 && styles.memoryIn)} aria-hidden="true">
            <span className={styles.memoryTitle}>translation memory</span>
            <span className={styles.memoryHit}>“Category-leading growth.” · 100%</span>
          </div>

          {/* Satellites: AI models (top), shown as the fan resolves. */}
          <div className={clsx(styles.models, fan && styles.modelsIn)} aria-hidden="true">
            {MODELS.map((m, idx) => (
              <ModelBadge key={m} name={m} style={{ transitionDelay: `${idx * 80}ms` }} />
            ))}
          </div>

          {/* The star. */}
          <div className={styles.slideWrap}>
            <SlideCard
              redacted={redacted}
              protectedTerms={locks}
              recycled={phase >= 2}
              tint={fan ? "green" : "en"}
              className={styles.slide}
            />
          </div>

          {/* The fan of reviewed languages, arcing out under the slide. */}
          <div className={clsx(styles.fan, fan && styles.fanIn)} aria-hidden="true">
            {LANGS.map((l, idx) => (
              <LangCard
                key={l.code}
                lang={l}
                className={styles.fanCard}
                style={reduced ? undefined : { transitionDelay: `${idx * 55}ms` }}
              />
            ))}
          </div>
        </div>
      </button>

      <p className={shell.caption} key={phase}>
        <strong>{PHASES[phase]}.</strong> {CAPTIONS[phase]}
      </p>

      <button type="button" className={shell.cta} onClick={onOpen}>
        Try it in your browser <ArrowRight size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
