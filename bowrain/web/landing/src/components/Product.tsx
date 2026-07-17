import { PenTool, ClipboardCheck, Fingerprint, Database, Zap, Plug } from "lucide-react";
import { t } from "@neokapi/kapi-react/runtime";
import { useReveal } from "../useReveal";

// Every claim here traces to a shipped code path (epic 011 claims discipline):
// preview formats = the four format.PreviewBuilder implementations; presence,
// not CRDT; automation actions per bowrain/server/automation.go; connectors
// per the server-side registry (wordpress/figma/hubspot + file/git).
const CAPABILITIES = [
  {
    icon: PenTool,
    title: t("A shared editor for every format"),
    body: t(
      "In-context visual preview for HTML, Markdown, MDX, and JSX; structured block editing for everything else — app strings, XLIFF, subtitles, office documents. Suggestions from memory, term highlights, and QA checks inline.",
    ),
    detail: [t("Visual preview · 4 formats"), t("Block editing · all formats"), t("Live presence")],
  },
  {
    icon: ClipboardCheck,
    title: t("Review with a memory"),
    body: t(
      "Block statuses, notes, and per-block history with rollback. A workspace audit log records who changed what — and can be cryptographically verified.",
    ),
    detail: [t("Draft → reviewed → approved"), t("History & rollback"), t("Verifiable audit log")],
  },
  {
    icon: Fingerprint,
    title: t("Brand voice, scored"),
    body: t(
      "Profiles hold tone, style, and vocabulary rules. Drafts score 0–100 across five dimensions; trends and drift show where a surface is sliding off voice.",
    ),
    detail: [t("Five-dimension score"), t("Drift detection"), t("Rules from corrections")],
  },
  {
    icon: Database,
    title: t("Terminology & translation memory"),
    body: t(
      "One termbase and one memory for the whole workspace, applied in every draft and every lookup. Import from CSV or JSON; entity-aware matching reuses past work even when names and numbers change.",
    ),
    detail: [t("Shared termbase & TM"), t("CSV / JSON import"), t("Entity-aware reuse")],
  },
  {
    icon: Zap,
    title: t("Automation that keeps pace"),
    body: t(
      "When content arrives, translation starts; reviewers get tasks; people get notified. Runs are visible with per-step logs, and the server runs the kapi loop, keeping every locale caught up.",
    ),
    detail: [t("Translate on push"), t("Review tasks & notifications"), t("Run logs")],
  },
  {
    icon: Plug,
    title: t("Connected to where content lives"),
    body: t(
      "WordPress, Figma, and HubSpot connectors sync content in and publish translations back. Files and git repositories connect through the server — or push and pull from any repo or CI with kapi.",
    ),
    detail: [t("WordPress · Figma · HubSpot"), t("Files & git"), t("kapi push / pull")],
  },
];

export function Product() {
  const ref = useReveal();

  return (
    <section id="product" className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            The work of staying on-brand, made visible.
          </h2>
          <p className="mt-3 text-muted-foreground">
            Everything you need to run the loop — editing, review, voice, memory, automation, and
            the systems your content already lives in.
          </p>
        </div>

        <div className="mt-12 grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {CAPABILITIES.map((c) => (
            <div
              key={c.title}
              className="flex flex-col rounded-xl border border-border bg-card p-6"
            >
              <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                <c.icon className="h-5 w-5 text-primary" />
              </div>
              <h3 translate="no" className="text-lg font-semibold">
                {c.title}
              </h3>
              <p className="mt-2 flex-1 text-sm leading-relaxed text-muted-foreground">{c.body}</p>
              <div className="mt-4 flex flex-wrap gap-1.5">
                {c.detail.map((d) => (
                  <span
                    key={d}
                    className="rounded-md bg-secondary px-2 py-0.5 text-xs text-secondary-foreground"
                  >
                    {d}
                  </span>
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
