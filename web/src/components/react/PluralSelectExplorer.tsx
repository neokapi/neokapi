import React, { useState } from "react";

import styles from "./PluralSelectExplorer.module.css";

// Locales chosen to show different CLDR plural-category sets:
// en (one/other), fr (one/many/other), pl (one/few/many/other),
// ar (zero/one/two/few/many/other), ja (other only).
const LOCALES = [
  { id: "en", label: "English (en)" },
  { id: "fr", label: "French (fr)" },
  { id: "pl", label: "Polish (pl)" },
  { id: "ar", label: "Arabic (ar)" },
  { id: "ja", label: "Japanese (ja)" },
];

// The forms a source author writes as <Plural> children. `#` is the count
// placeholder. These mirror the messages example used above on this page.
const FORMS: Record<string, string> = {
  zero: "No messages",
  one: "1 message",
  two: "# messages",
  few: "# messages",
  many: "# messages",
  other: "# messages",
};

const TAG: Record<string, string> = {
  zero: "<Zero>",
  one: "<One>",
  two: "<Two>",
  few: "<Few>",
  many: "<Many>",
  other: "<Other>",
};

/**
 * PluralSelectExplorer shows the source-React ↔ translator-ICU duality live:
 * move the count / change the locale and watch which CLDR category
 * `Intl.PluralRules` resolves (the same lookup `<Plural>` does at render time),
 * alongside the canonical ICU template the extractor emits for translators.
 * Dependency-free (native Intl), so it renders the same on the server and client.
 */
export function PluralSelectExplorer(): React.ReactElement {
  const [count, setCount] = useState(1);
  const [locale, setLocale] = useState("en");

  const rules = new Intl.PluralRules(locale);
  const category = rules.select(count);
  const used = rules.resolvedOptions().pluralCategories;

  const rendered = (FORMS[category] ?? FORMS.other).replace("#", String(count));
  const icu =
    "{count, plural, " + used.map((c) => `${c} {${FORMS[c] ?? FORMS.other}}`).join(" ") + "}";

  return (
    <div className={styles.explorer}>
      <div className={styles.controls}>
        <label className={styles.control}>
          <span>
            count: <strong>{count}</strong>
          </span>
          <input
            type="range"
            min={0}
            max={12}
            value={count}
            onChange={(e) => setCount(Number(e.target.value))}
            aria-label="count"
          />
        </label>
        <label className={styles.control}>
          <span>locale</span>
          <select value={locale} onChange={(e) => setLocale(e.target.value)} aria-label="locale">
            {LOCALES.map((l) => (
              <option key={l.id} value={l.id}>
                {l.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className={styles.row}>
        <div className={styles.col}>
          <div className={styles.colLabel}>Rendered — the source author&apos;s React</div>
          <div className={styles.rendered}>{rendered}</div>
          <div className={styles.hint}>
            <code>Intl.PluralRules(&quot;{locale}&quot;)</code> selects <code>{category}</code> →{" "}
            <code>{TAG[category]}</code>
          </div>
        </div>
        <div className={styles.col}>
          <div className={styles.colLabel}>Emitted ICU — the translator&apos;s CAT tool</div>
          <pre className={styles.icu}>
            <code>{icu}</code>
          </pre>
          <div className={styles.hint}>
            {LOCALES.find((l) => l.id === locale)?.label} uses {used.join(", ")}.
          </div>
        </div>
      </div>
    </div>
  );
}

export default PluralSelectExplorer;
