import { AppWindow, Globe2, BookOpen, Rss, Mail, MessageCircle } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Logo } from "./Logo";
import { useReveal } from "../useReveal";

// One brand, many surfaces: each surface is a project in the workspace; all of
// them draw on the same brand profiles, terminology, and content memory.
const SURFACES = [
  { icon: AppWindow, label: t("App strings"), note: t("JSON, XLIFF, mobile formats") },
  { icon: Globe2, label: t("Website"), note: t("HTML, Markdown, CMS") },
  { icon: BookOpen, label: t("Docs"), note: t("Markdown, MDX") },
  { icon: Rss, label: t("Blog"), note: t("long-form, per-author") },
  { icon: Mail, label: t("Email"), note: t("templates, campaigns") },
  { icon: MessageCircle, label: t("Posts"), note: t("social, announcements") },
];

export function Surfaces() {
  const ref = useReveal();

  return (
    <section className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            A profile is a named set of <span className="prism-text">coordinates.</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            Who it is for, where it appears, in what register, for which market, and from when until
            when. A profile bundles those five and gives them a name — developer reference, end-user
            help, regulated disclosure. Coordinates are declared where they are cheapest to declare
            and inherited from there: a folder carries a profile for everything inside it, a file
            can override its folder, and a passage can override its file. Nobody tags a paragraph
            unless a paragraph is genuinely the exception.
          </p>
        </div>

        <div className="mt-12 grid items-center gap-8 lg:grid-cols-[1fr_auto_1fr]">
          {/* Left: three surfaces */}
          <div className="grid gap-3">
            {SURFACES.slice(0, 3).map((s) => (
              <div
                key={s.label}
                className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3"
              >
                <s.icon className="h-4 w-4 text-primary" />
                <span translate="no" className="text-sm font-medium">
                  {s.label}
                </span>
                <span className="ml-auto text-xs text-muted-foreground">{s.note}</span>
              </div>
            ))}
          </div>

          {/* Center: the shared core */}
          <div className="mx-auto flex flex-col items-center gap-3 rounded-2xl border border-border bg-card px-8 py-6 text-center shadow-lg shadow-primary/5">
            <Logo className="h-12 w-12" />
            <div className="text-sm font-semibold">One shared core</div>
            <div className="text-xs text-muted-foreground">
              brand voice · terminology
              <br />
              content memory · review rules
            </div>
          </div>

          {/* Right: three surfaces */}
          <div className="grid gap-3">
            {SURFACES.slice(3).map((s) => (
              <div
                key={s.label}
                className="flex items-center gap-3 rounded-xl border border-border bg-card px-4 py-3"
              >
                <s.icon className="h-4 w-4 text-primary" />
                <span translate="no" className="text-sm font-medium">
                  {s.label}
                </span>
                <span className="ml-auto text-xs text-muted-foreground">{s.note}</span>
              </div>
            ))}
          </div>
        </div>

        <p className="mx-auto mt-8 max-w-2xl text-center text-sm text-muted-foreground">
          A correction accepted in one project teaches the whole workspace: the memory, the terms,
          and the promoted rules apply wherever that content appears next.
        </p>
      </div>
    </section>
  );
}
