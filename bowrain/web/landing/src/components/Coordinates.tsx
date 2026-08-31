import { useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_HOW } from "../sections";
import { captureLandingEvent } from "../analytics";
import { markEngaged } from "../sectionSignals";

// The mechanism section, and the one the page is a test of: does a stranger
// follow "coordinates" without a demo?
//
// The rename above proved that where the words sit decides the rule. This
// section says what "where" is made of, and then shows the answer a real tool
// gives: the three examples below are the three rows of that table, resolved.
// The example switcher is the comprehension signal: a reader who compares two
// points has understood that a point is a thing you can compare.

interface Point {
  /** Coordinate slug, the shape a profile/channel binding actually takes. */
  point: string;
  tab: string;
  path: string;
  profile: string;
  channel: string;
  collection: string;
  voice: string;
  /** The rule the rename table set for this surface, as resolved here. */
  rule: string;
  term: string;
}

const POINTS: Point[] = [
  {
    point: "docs/help",
    tab: t("Help centre"),
    path: "help/account/rename-a-workspace.md",
    profile: "docs",
    channel: "help",
    collection: "help-centre",
    voice: t("end-user help"),
    rule: t("Use the new name."),
    term: t("Workspace: preferred. The old name is discouraged here."),
  },
  {
    point: "docs/migration",
    tab: t("Migration guide"),
    path: "help/migrate/2026-rename.md",
    profile: "docs",
    channel: "migration",
    collection: "migration-guide",
    voice: t("end-user help"),
    rule: t("Both names are permitted, side by side."),
    term: t("Workspace: preferred. Project: permitted until 2027-01-01."),
  },
  {
    point: "product/api",
    tab: t("API reference"),
    path: "reference/api/projects.mdx",
    profile: "product",
    channel: "api",
    collection: "api-reference",
    voice: t("developer reference"),
    rule: t("The old name must not change."),
    term: t("project: required; it is the wire field, not a word."),
  },
];

const AXES = [
  { axis: t("audience"), value: t("who it is for") },
  { axis: t("surface"), value: t("where it appears") },
  { axis: t("register"), value: t("how formal it reads") },
  { axis: t("market"), value: t("which market it serves") },
  { axis: t("validity"), value: t("from when until when") },
];

const LADDER = [
  {
    scope: t("A project"),
    body: t(
      "The recipe declares the axes every collection inherits: the brand, the kind of document, whatever else the content varies along. Declared once, at the broadest scope that is true.",
    ),
  },
  {
    scope: t("A collection"),
    body: t(
      "This folder is the help centre; that one is the API reference. The collection carries the profile for everything inside it, overriding the project on the one axis it differs on and inheriting the rest.",
    ),
  },
  {
    scope: t("An exception"),
    body: t(
      "One document that belongs to the folder but not to its register, a deprecation notice inside the help centre, is given a collection of its own. Nobody carves out an exception unless it genuinely is one.",
    ),
  },
];

export function Coordinates() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_HOW);
  const ref = useReveal();
  const [active, setActive] = useState(0);
  const point = POINTS[active];

  function selectPoint(index: number) {
    setActive(index);
    // The one explicit signal for the concept: comparing two points is what
    // understanding "coordinates" looks like from outside.
    markEngaged("coordinates");
    captureLandingEvent("coordinates_explored", {
      point: POINTS[index].point,
      index,
    });
  }

  return (
    <section id="how" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-border px-3 py-1 font-mono text-xs text-muted-foreground">
            WHERE THE WORDS SIT
          </div>
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("Content has coordinates.")}{" "}
            <span className="prism-text">{t("Rules follow them.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "A profile is a named bundle of coordinates: who the content is for, where it appears, in what register, for which market, and from when until when. Bind a profile to a place and everything in that place is governed from there.",
            )}
          </p>
        </div>

        {/* The five axes a profile is made of. */}
        <dl className="mt-12 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-3 lg:grid-cols-5">
          {AXES.map((a) => (
            <div key={a.axis} className="bg-card px-4 py-5">
              <dt
                translate="no"
                className="font-mono text-xs uppercase tracking-wider text-primary"
              >
                {a.axis}
              </dt>
              <dd className="mt-1.5 text-sm text-muted-foreground">{a.value}</dd>
            </div>
          ))}
        </dl>

        <div className="mt-12 grid gap-8 lg:grid-cols-[minmax(0,22rem)_1fr]">
          {/* Inheritance: declared where it is cheapest, overridden where it must be. */}
          <div>
            <h3 className="text-base font-semibold">{t("Declared once, inherited down")}</h3>
            <ol className="mt-5 space-y-5 border-l border-border pl-5">
              {LADDER.map((step, i) => (
                <li key={step.scope} className="relative">
                  <span
                    aria-hidden="true"
                    className="absolute -left-[1.4rem] mt-1.5 size-2 rounded-full bg-primary"
                    style={{ opacity: 1 - i * 0.25 }}
                  />
                  <div className="text-sm font-medium">{step.scope}</div>
                  <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{step.body}</p>
                </li>
              ))}
            </ol>
          </div>

          {/* The answer a real tool gives at a real point. */}
          <div className="self-start overflow-hidden rounded-xl border border-border bg-card">
            <div className="flex flex-wrap gap-1 border-b border-border px-3 py-2">
              {POINTS.map((p, i) => (
                <button
                  key={p.point}
                  type="button"
                  onClick={() => selectPoint(i)}
                  aria-pressed={i === active}
                  className={`rounded-md px-3 py-1 text-xs transition ${
                    i === active
                      ? "bg-primary/10 text-primary"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {p.tab}
                </button>
              ))}
            </div>
            <div className="overflow-x-auto p-4 font-mono text-xs leading-relaxed sm:text-[13px]">
              <div translate="no" className="whitespace-nowrap text-muted-foreground">
                <span className="text-success">$</span> kapi context {point.path}
              </div>
              <div translate="no" className="mt-3 whitespace-nowrap">
                Point <span className="text-primary">{point.point}</span>: profile{" "}
                <span className="text-primary">{point.profile}</span>, channel{" "}
                <span className="text-primary">{point.channel}</span>, collection{" "}
                <span className="text-primary">{point.collection}</span>.
              </div>
              <div className="mt-1 text-muted-foreground">
                {t("Voice")} <span className="text-foreground">{point.voice}</span>.
              </div>
              <div className="mt-3 text-muted-foreground">## {t("Rule in force")}</div>
              <div className="mt-1">{point.rule}</div>
              <div className="mt-3 text-muted-foreground">## {t("Terms in force")}</div>
              <div className="mt-1">{point.term}</div>
            </div>
          </div>
        </div>

        <p className="mx-auto mt-10 max-w-2xl text-center text-muted-foreground">
          {t(
            "The agent asking what applies here, the check asking whether a rule fires, and the engine asking whether approved wording may be reused are one question with three callers. Coordinates are what make it answerable.",
          )}
        </p>
      </div>
    </section>
  );
}
