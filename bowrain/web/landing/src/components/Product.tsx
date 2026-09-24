import { PenTool, ClipboardCheck, Fingerprint, Database, Zap, Plug } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { useReveal } from "../useReveal";
import { useSectionSignals } from "../useSectionSignals";
import { SECTION_PRODUCT } from "../sections";

// Preview support follows format.PreviewBuilder implementations. Automation
// actions and connector availability follow the server registries.
const CAPABILITIES = [
  {
    icon: PenTool,
    title: t("A shared content editor"),
    body: t(
      "In-context visual preview for HTML, Markdown, MDX, and JSX; structured block editing for everything else: app strings, subtitles, office documents, interchange files. Suggestions from memory, term highlights, and checks inline.",
    ),
    detail: [t("Visual preview · web formats"), t("Structured block editing"), t("Live presence")],
  },
  {
    icon: ClipboardCheck,
    title: t("Review history"),
    body: t(
      "Block statuses, notes, and per-block history with rollback. A workspace audit log records who changed what, and can be cryptographically verified.",
    ),
    detail: [t("Draft → reviewed → approved"), t("History & rollback"), t("Verifiable audit log")],
  },
  {
    icon: Fingerprint,
    title: t("Writing guidance and findings"),
    body: t(
      "A voice profile holds writing guidance and wording rules for its scope. Inspect the findings from configured checks; their scores describe the checks performed, not overall content quality.",
    ),
    detail: [t("Configured checks"), t("Drift detection"), t("Rules from corrections")],
  },
  {
    icon: Database,
    title: t("Terms and content memory"),
    body: t(
      "Share a terms store and content memory across the workspace. Retrieve eligible wording for the project and inspect its source before accepting a suggestion.",
    ),
    detail: [t("Shared terms & content memory"), t("CSV / JSON import"), t("Scoped reuse")],
  },
  {
    icon: Zap,
    title: t("Content automation"),
    body: t(
      "Configure incoming content to trigger drafting, review tasks and notifications. Inspect run status and per-step logs to track progress.",
    ),
    detail: [t("Draft on push"), t("Review tasks & notifications"), t("Run logs")],
  },
  {
    icon: Plug,
    title: t("Content connectors"),
    body: t(
      "WordPress, Figma and HubSpot connectors import content and publish approved text. Connect a GitHub or GitLab repository, or sync a local checkout with kapi.",
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
              "Edit and review content across projects, share terms and content memory, and connect the systems where your team works.",
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
