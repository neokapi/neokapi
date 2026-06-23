import React from "react";
import clsx from "clsx";
import { Check, Lock, RotateCcw, Sparkles } from "lucide-react";
import { SLIDE, type Bullet, type Lang } from "./heroData";
import styles from "./parts.module.css";

// Shared visual primitives for the three animated hero prototypes. The SlideCard
// is the composed pitch slide (content centred, never crammed in a corner); the
// chips/cards are the moving pieces the concepts choreograph.

export type Tint = "en" | "qps" | "green";

export interface SlideCardProps {
  /** Replace the title (e.g. a localized headline). */
  title?: React.ReactNode;
  /** Redaction param keys whose values are masked. */
  redacted?: string[];
  /** Terms shown with a protection lock. */
  protectedTerms?: string[];
  /** The editable claim shows a "from memory" recycle marker. */
  recycled?: boolean;
  /** Background/accent tint. */
  tint?: Tint;
  /** Hide the footer band (compact contexts). */
  noFooter?: boolean;
  className?: string;
}

function Value({ bullet, masked }: { bullet: Bullet; masked: boolean }): React.ReactElement {
  return (
    <span className={styles.bulletRow}>
      <span className={styles.bulletLabel}>{bullet.label}</span>{" "}
      <span className={clsx(styles.value, masked && styles.valueMasked)}>
        <span className={styles.valueText}>{bullet.value}</span>
      </span>
    </span>
  );
}

// The pitch slide. Content is vertically centred between a top kicker and a
// bottom footer, with the title large and a faint growth motif filling the
// right — so the copy reads as a full slide, not a corner of one.
export function SlideCard({
  title,
  redacted = [],
  protectedTerms = [],
  recycled,
  tint = "en",
  noFooter,
  className,
}: SlideCardProps): React.ReactElement {
  const masked = new Set(redacted);
  return (
    <div className={clsx(styles.slide, className)} data-tint={tint}>
      <div className={styles.slideChrome}>
        <span className={styles.slideFile}>
          <span className={styles.slideDot} aria-hidden="true" />
          {SLIDE.file}
        </span>
      </div>

      <svg className={styles.slideArt} viewBox="0 0 120 80" aria-hidden="true">
        <rect x="2" y="50" width="16" height="30" rx="3" />
        <rect x="27" y="38" width="16" height="42" rx="3" />
        <rect x="52" y="26" width="16" height="54" rx="3" />
        <rect x="77" y="15" width="16" height="65" rx="3" />
        <rect x="102" y="3" width="16" height="77" rx="3" />
      </svg>

      <div className={styles.slideBody}>
        <span className={styles.kicker}>{SLIDE.kicker}</span>
        <div className={styles.slideMain}>
          <h3 className={styles.title}>{title ?? SLIDE.title}</h3>
          <div className={styles.bullets}>
            {SLIDE.bullets.map((b) =>
              b.text ? (
                <span key={b.id} className={clsx(styles.bulletRow, recycled && styles.recycled)}>
                  {b.text}
                  {recycled && (
                    <span className={styles.recycleTag}>
                      <RotateCcw size={10} strokeWidth={2.5} aria-hidden="true" />
                      from memory
                    </span>
                  )}
                </span>
              ) : (
                <Value key={b.id} bullet={b} masked={masked.has(b.redactKey ?? "")} />
              ),
            )}
          </div>
        </div>
      </div>

      {!noFooter && (
        <div className={styles.slideFoot}>
          <span>{SLIDE.footerLeft}</span>
          <span className={styles.confidential}>{SLIDE.footerRight}</span>
        </div>
      )}

      {protectedTerms.length > 0 && (
        <div className={styles.lockRow} aria-hidden="true">
          {protectedTerms.map((t) => (
            <span key={t} className={styles.lockPill}>
              <Lock size={9} strokeWidth={2.5} />
              {t}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}

export function TermChip({
  term,
  domain,
  locked,
  className,
  style,
}: {
  term: string;
  domain?: string;
  locked?: boolean;
  className?: string;
  style?: React.CSSProperties;
}): React.ReactElement {
  return (
    <span
      className={clsx(styles.termChip, locked && styles.termChipLocked, className)}
      style={style}
    >
      {locked && <Lock size={10} strokeWidth={2.5} aria-hidden="true" />}
      <span className={styles.termName}>{term}</span>
      {domain && <span className={styles.termDomain}>{domain}</span>}
    </span>
  );
}

export function ParamTag({
  children,
  active,
  className,
  style,
}: {
  children: React.ReactNode;
  active?: boolean;
  className?: string;
  style?: React.CSSProperties;
}): React.ReactElement {
  return (
    <span
      className={clsx(styles.paramTag, active && styles.paramTagActive, className)}
      style={style}
    >
      {children}
    </span>
  );
}

export function ModelBadge({
  name,
  className,
  style,
}: {
  name: string;
  className?: string;
  style?: React.CSSProperties;
}): React.ReactElement {
  return (
    <span className={clsx(styles.modelBadge, className)} style={style}>
      <Sparkles size={11} strokeWidth={2.5} aria-hidden="true" />
      {name}
    </span>
  );
}

export function LangCard({
  lang,
  className,
  style,
}: {
  lang: Lang;
  className?: string;
  style?: React.CSSProperties;
}): React.ReactElement {
  return (
    <div
      className={clsx(styles.langCard, lang.reused && styles.langCardReused, className)}
      style={style}
      dir={lang.rtl ? "rtl" : undefined}
    >
      <span className={styles.langLabel}>{lang.label}</span>
      <span className={styles.langTitle}>{lang.title}</span>
      <span className={clsx(styles.langCheck, lang.reused && styles.langCheckReused)}>
        <Check size={10} strokeWidth={3} aria-hidden="true" />
        {lang.reused ? "reused" : "reviewed"}
      </span>
    </div>
  );
}
