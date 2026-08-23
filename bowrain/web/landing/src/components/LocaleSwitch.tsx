import { Languages as LanguagesIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";

// Each label is written IN its own locale, which is the convention every
// language menu follows: a reader looking for their language recognises it
// written the way they write it, not translated into a language they do not
// read. For the pseudo-locale that means the label is itself pseudo-mangled —
// "Þšéüđö Éñĝļîšĥ" — so the entry demonstrates the transformation it selects
// before you click it. Labels are therefore NOT passed through t().
export function LocaleSwitch() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click and on Escape: a menu that traps focus or lingers
  // after the pointer leaves is the usual defect in a hand-rolled dropdown.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  if (__LOCALES__.length < 2) return null;

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        // The icon carries no text, so the accessible name is the only thing a
        // screen reader has. This one string IS translated: it describes the
        // control, not a language.
        aria-label={t("Change language")}
        className="flex h-8 w-8 items-center justify-center rounded-lg text-muted-foreground transition hover:bg-secondary hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
      >
        <LanguagesIcon className="h-4 w-4" aria-hidden />
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 z-50 mt-2 min-w-44 rounded-md border border-border bg-background py-1 shadow-lg"
        >
          {__LOCALES__.map((l) => (
            <a
              key={l.code}
              href={l.href}
              role="menuitem"
              // lang tells a screen reader how to pronounce the label. qps is
              // English text that has been mangled, so it stays "en" — claiming
              // otherwise would make a screen reader attempt a language that
              // does not exist.
              lang={l.code === "qps" ? "en" : l.code}
              aria-current={l.code === __LOCALE__ ? "true" : undefined}
              className={
                "block px-3 py-1.5 text-sm transition-colors hover:bg-secondary " +
                (l.code === __LOCALE__ ? "font-medium text-foreground" : "text-muted-foreground")
              }
            >
              {l.label}
            </a>
          ))}
        </div>
      )}
    </div>
  );
}
