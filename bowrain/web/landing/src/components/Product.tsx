import { PenTool, ClipboardCheck, Fingerprint, Database, Zap, Plug } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_PRODUCT } from "../sections";

// Every claim here traces to a shipped code path (epic 011 claims discipline):
// preview formats = the four format.PreviewBuilder implementations; presence,
// not CRDT; automation actions per bowrain/server/automation.go; connectors
// per the server-side registry (wordpress/figma/hubspot + file/git).
const CAPABILITIES = [
  {
    icon: PenTool,
    title: t("A shared editor for every format"),
    body: t(
      "In-context visual preview for HTML, Markdown, MDX, and JSX; structured block editing for everything else: app strings, subtitles, office documents, interchange files. Suggestions from memory, term highlights, and checks inline.",
    ),
    detail: [
      t("Visual preview · web formats"),
      t("Block editing · all formats"),
      t("Live presence"),
    ],
  },
  {
    icon: ClipboardCheck,
    title: t("Review with a memory"),
    body: t(
      "Block statuses, notes, and per-block history with rollback. A workspace audit log records who changed what, and can be cryptographically verified.",
    ),
    detail: [t("Draft → reviewed → approved"), t("History & rollback"), t("Verifiable audit log")],
  },
  {
    icon: Fingerprint,
    title: t("Voice profile, scored"),
    body: t(
      "A profile holds the tone, style, and vocabulary rules that apply at its coordinates. Drafts score 0–100 across five dimensions; trends and drift show where a surface is sliding off profile.",
    ),
    detail: [t("Five-dimension score"), t("Drift detection"), t("Rules from corrections")],
  },
  {
    icon: Database,
    title: t("Terms and content memory"),
    body: t(
      "One terms store and one content memory for the whole workspace, applied in every draft and every lookup. Import from CSV or JSON; entity-aware matching recycles approved wording even when names and numbers change.",
    ),
    detail: [
      t("Shared terms & content memory"),
      t("CSV / JSON import"),
      t("Entity-aware recycling"),
    ],
  },
  {
    icon: Zap,
    title: t("Automation that keeps pace"),
    body: t(
      "When content arrives, drafting starts; reviewers get tasks; people get notified. Runs are visible with per-step logs, so the state of any piece of content is a thing you can look up rather than ask about.",
    ),
    detail: [t("Draft on push"), t("Review tasks & notifications"), t("Run logs")],
  },
  {
    icon: Plug,
    title: t("Connected to where content lives"),
    body: t(
      "WordPress, Figma, and HubSpot connectors sync content in and publish approved text back. A GitHub or GitLab repository connects with no pipeline at all, or a developer drives it from their own checkout with kapi.",
    ),
    detail: [t("WordPress · Figma · HubSpot"), t("GitHub · GitLab"), t("kapi (developer & CI)")],
  },
];

export function Product() {
  const sectionRef = useSectionSignals<HTMLElement>(SECTION_PRODUCT);
  const ref = useReveal();

  return (
    <section id="product" ref={sectionRef} className="mx-auto max-w-6xl px-6 py-24">
      <div ref={ref} className="reveal">
        <div className="mx-auto max-w-3xl text-center">
          <h2 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">
            {t("What the workspace")} <span className="prism-text">{t("holds.")}</span>
          </h2>
          <p className="mt-3 text-muted-foreground">
            {t(
              "One editor, one review trail, one terms store and one content memory, shared by every project in the workspace, and the connectors that reach the systems your content already lives in.",
            )}
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
