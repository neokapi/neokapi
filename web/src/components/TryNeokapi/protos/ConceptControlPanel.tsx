import React from "react";
import clsx from "clsx";
import { ArrowRight, ChevronRight } from "lucide-react";
import { LANGS, MODELS, REDACT_PARAMS, TERMS, TM_HIT } from "./heroData";
import { LangCard, ModelBadge, ParamTag, SlideCard, TermChip } from "./parts";
import { useTimeline } from "./useTimeline";
import shell from "./shell.module.css";
import styles from "./ConceptControlPanel.module.css";

// Concept B — Control panel drives the slide. A parameter deck on the left lights
// up row by row; the slide on the right reacts live to each. Reads as "this is
// configurable": redaction params, the termbase, translation memory, and the AI
// models are all knobs, and you watch the document respond.

const ROWS = ["Redaction", "Termbase", "Memory", "Models"] as const;
const CAPTIONS = [
  "Redact by policy — sensitive figures never reach a model.",
  "Feed the termbase — protected terms held verbatim, everywhere.",
  "Recycle translation memory — only pay to translate what changed.",
  "Call AI across every language — human-reviewed, written back byte-for-byte.",
];
const DURATIONS = [2600, 2800, 2600, 3800];

function Row({
  label,
  active,
  done,
  children,
}: {
  label: string;
  active: boolean;
  done: boolean;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div className={clsx(styles.row, active && styles.rowActive, done && styles.rowDone)}>
      <span className={styles.rowLabel}>
        <span className={styles.rowDot} aria-hidden="true" />
        {label}
      </span>
      <div className={styles.rowBody}>{children}</div>
      <ChevronRight className={styles.rowArrow} size={14} strokeWidth={2.5} aria-hidden="true" />
    </div>
  );
}

export default function ConceptControlPanel({
  onOpen,
}: {
  onOpen: () => void;
}): React.ReactElement {
  const { phase, setPhase, reduced, hoverBind } = useTimeline(4, DURATIONS);
  const fan = phase === 3;

  return (
    <div className={shell.card} {...hoverBind}>
      <div className={styles.stage}>
        {/* Control deck. */}
        <div className={styles.panel} role="tablist" aria-label="kapi parameters">
          <span className={styles.panelTitle}>parameters</span>

          <button
            type="button"
            role="tab"
            aria-selected={phase === 0}
            className={styles.rowBtn}
            onClick={() => setPhase(0)}
          >
            <Row label={ROWS[0]} active={phase === 0} done={phase > 0}>
              {REDACT_PARAMS.map((p) => (
                <ParamTag key={p.key} active={phase >= 0}>
                  {p.label}
                </ParamTag>
              ))}
            </Row>
          </button>

          <button
            type="button"
            role="tab"
            aria-selected={phase === 1}
            className={styles.rowBtn}
            onClick={() => setPhase(1)}
          >
            <Row label={ROWS[1]} active={phase === 1} done={phase > 1}>
              {TERMS.map((t) => (
                <TermChip key={t.term} term={t.term} locked={phase >= 1} />
              ))}
            </Row>
          </button>

          <button
            type="button"
            role="tab"
            aria-selected={phase === 2}
            className={styles.rowBtn}
            onClick={() => setPhase(2)}
          >
            <Row label={ROWS[2]} active={phase === 2} done={phase > 2}>
              <span className={clsx(styles.memChip, phase >= 2 && styles.memChipOn)}>
                {TM_HIT.match} match
              </span>
            </Row>
          </button>

          <button
            type="button"
            role="tab"
            aria-selected={phase === 3}
            className={styles.rowBtn}
            onClick={() => setPhase(3)}
          >
            <Row label={ROWS[3]} active={phase === 3} done={false}>
              {MODELS.map((m) => (
                <ModelBadge key={m} name={m} className={clsx(!fan && styles.dim)} />
              ))}
            </Row>
          </button>
        </div>

        {/* Live screen — the slide reacting, then the language fan. */}
        <button
          type="button"
          className={styles.screen}
          onClick={onOpen}
          aria-label="Open the live demo"
        >
          <div className={clsx(styles.screenSlide, fan && styles.screenSlideOut)}>
            <SlideCard
              redacted={REDACT_PARAMS.map((p) => p.key)}
              protectedTerms={phase >= 1 ? TERMS.map((t) => t.term) : []}
              recycled={phase >= 2}
              tint={fan ? "green" : "en"}
              noFooter
            />
          </div>
          <div className={clsx(styles.langs, fan && styles.langsIn)} aria-hidden="true">
            {LANGS.map((l, idx) => (
              <LangCard
                key={l.code}
                lang={l}
                className={styles.langCard}
                style={reduced ? undefined : { transitionDelay: `${idx * 55}ms` }}
              />
            ))}
          </div>
        </button>
      </div>

      <p className={shell.caption} key={phase}>
        <strong>{ROWS[phase]}.</strong> {CAPTIONS[phase]}
      </p>

      <button type="button" className={shell.cta} onClick={onOpen}>
        Try it in your browser <ArrowRight size={16} aria-hidden="true" />
      </button>
    </div>
  );
}
