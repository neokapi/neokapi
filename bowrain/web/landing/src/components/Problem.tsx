import { GitCompareArrows, Megaphone, SearchCheck } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";

const CARDS = [
  {
    icon: GitCompareArrows,
    color: "text-prism-4",
    bg: "bg-prism-4/10",
    title: t("Languages drift"),
    body: t(
      "The source changes daily; translations lag, fall back to English, or quietly go stale. Nobody can say which locale is actually current.",
    ),
  },
  {
    icon: Megaphone,
    color: "text-prism-2",
    bg: "bg-prism-2/10",
    title: t("Voice erodes"),
    body: t(
      "The app, the site, the blog, the release notes — each surface drifts its own way. Forbidden terms creep back in; tone flattens into generic AI prose.",
    ),
  },
  {
    icon: SearchCheck,
    color: "text-prism-3",
    bg: "bg-prism-3/10",
    title: t("Review doesn't scale"),
    body: t(
      "Spot checks and pasted diffs catch some of it. But the same correction gets made twenty times, because nothing remembers it was ever made.",
    ),
  },
];

export function Problem() {
  const ref = useReveal();

  return (
    <section className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            AI made content cheap to produce — and hard to keep right.
          </h2>
          <p className="mt-3 text-muted-foreground">
            Everyone now generates more copy, in more languages, on more surfaces than they can
            read. The bottleneck moved from writing to trusting what was written.
          </p>
        </div>

        <div className="mt-12 grid gap-6 md:grid-cols-3">
          {CARDS.map((c) => (
            <div key={c.title} className="rounded-xl border border-border bg-card p-6">
              <div className={`mb-4 flex h-10 w-10 items-center justify-center rounded-lg ${c.bg}`}>
                <c.icon className={`h-5 w-5 ${c.color}`} />
              </div>
              <h3 translate="no" className="text-lg font-semibold">
                {c.title}
              </h3>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{c.body}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
