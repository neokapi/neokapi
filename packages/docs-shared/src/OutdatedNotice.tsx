import "./OutdatedNotice.css";

/**
 * Why a recording no longer matches the current release.
 *
 * The two kinds are deliberately distinct, because they license very different
 * claims. `"interface"` says the footage shows screens that no longer exist —
 * a viewer following along will not find what they see. `"wording"` says only
 * that the narration uses retired terminology; every screen and every step is
 * still current. Telling a viewer the interface changed when it did not is the
 * failure mode this split exists to prevent.
 */
export type OutdatedKind = "interface" | "wording";

export interface OutdatedNoticeProps {
  kind: OutdatedKind;
  /**
   * An optional sentence naming the specific drift, e.g. "The Termbases
   * section is now Terms." Pages supply this; the component knows nothing
   * about which recordings are stale.
   */
  note?: string;
  /** id for the notice text, so a sibling player can `aria-describedby` it. */
  textId?: string;
}

const COPY: Record<OutdatedKind, { label: string; body: string }> = {
  interface: {
    label: "Outdated recording",
    body: "The interface has changed since this was recorded. The workflow it demonstrates still applies, but some screens and labels differ from the current release.",
  },
  wording: {
    label: "Outdated wording",
    body: "The narration uses terminology that has since been retired. The interface and the behaviour shown are unchanged.",
  },
};

/**
 * A banner marking a walkthrough recording as out of date, rendered above the
 * player so the recording keeps its demonstration value instead of being
 * pulled.
 *
 * Theme-aware through Docusaurus's `data-theme` attribute rather than
 * `useColorMode()`, for the same reason `ThemedVideo` is: that hook resolves to
 * a second module instance for workspace-package consumers and throws outside
 * the provider. CSS toggling renders during SSG and needs no React context.
 *
 * The status is real text inside a `role="note"` landmark — not a colour cue or
 * a decorative glyph — so a screen reader announces it while reading the page.
 * `role="alert"` would be wrong here: nothing changed dynamically, and it would
 * interrupt.
 */
export default function OutdatedNotice({ kind, note, textId }: OutdatedNoticeProps) {
  const { label, body } = COPY[kind];
  return (
    <div className="outdated-notice" role="note" aria-label={label}>
      <span className="outdated-notice__label">{label}</span>
      <p className="outdated-notice__body" id={textId}>
        {body}
        {note ? ` ${note}` : null}
      </p>
    </div>
  );
}
