import { FileText, Languages, ShieldCheck, Wand2, Repeat, Workflow } from "lucide-react";

const STEPS = [
  {
    n: "01",
    icon: FileText,
    title: "Extract",
    cmd: "kapi extract report.docx",
    description:
      "Pull translatable text out of any format — DOCX, JSON, XLIFF, Markdown and more — into a clean structured view. The original structure, styles, and placeholders are remembered for a faithful write-back.",
    accent: "text-brand-400",
    bg: "bg-brand-500/8",
    border: "border-brand-500/15",
  },
  {
    n: "02",
    icon: Languages,
    title: "Translate",
    cmd: "kapi ai-translate · or via MCP",
    description:
      "Translate with AI or MT, or let your assistant draft and edit content through MCP — placeholders, inline tags, and markup preserved as it goes.",
    accent: "text-accent-cyan",
    bg: "bg-accent-cyan/8",
    border: "border-accent-cyan/15",
  },
  {
    n: "03",
    icon: ShieldCheck,
    title: "Check",
    cmd: "kapi verify",
    description:
      "Run the checks like tests: do-not-translate, placeholder and tag integrity, terminology, and brand voice. Findings are specific and actionable — the exact strings and rules that broke.",
    accent: "text-accent-amber",
    bg: "bg-accent-amber/8",
    border: "border-accent-amber/15",
  },
  {
    n: "04",
    icon: Wand2,
    title: "Fix",
    cmd: "apply suggestions · re-run",
    description:
      "Resolve a dropped placeholder, a translated product name, or an off-voice phrase — keeping meaning intact — then re-run the checks until they pass.",
    accent: "text-accent-rose",
    bg: "bg-accent-rose/8",
    border: "border-accent-rose/15",
  },
  {
    n: "05",
    icon: Repeat,
    title: "Write back",
    cmd: "kapi merge",
    description:
      "Write the result into the original native format, unchanged except where you intended — a byte-faithful round-trip, in every locale (with more formats through the okapi-bridge).",
    accent: "text-forest-400",
    bg: "bg-forest-400/8",
    border: "border-forest-400/15",
  },
  {
    n: "06",
    icon: Workflow,
    title: "Gate",
    cmd: "kapi verify  (in CI)",
    description:
      "Keep recipes, profiles, and termbases version-controlled and gate quality on every commit — the same checks locally and in your pipeline, exiting non-zero on failure.",
    accent: "text-brand-400",
    bg: "bg-brand-500/8",
    border: "border-brand-500/15",
  },
];

export function BrandLoop() {
  return (
    <section id="brand-loop" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-brand-500/15 bg-brand-500/[0.06] px-3.5 py-1.5 text-xs font-medium text-brand-300">
            The pipeline
          </div>
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            From any file to{" "}
            <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
              shipped, faithfully, in every language
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            One engine spans the whole pipeline: extract from any format, translate and check the
            content like you test code, then write the original back — faithfully.
          </p>
        </div>

        {/* Step rail */}
        <div className="mb-10 hidden items-center justify-between gap-1 lg:flex">
          {STEPS.map((s, i) => (
            <div key={s.n} className="flex items-center gap-1">
              <span className={`font-mono text-xs font-semibold ${s.accent}`}>{s.title}</span>
              {i < STEPS.length - 1 && <span className="mx-1 text-surface-600">&rarr;</span>}
            </div>
          ))}
        </div>

        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {STEPS.map((s) => (
            <div
              key={s.n}
              className={`group relative rounded-2xl border ${s.border} ${s.bg} p-6 transition-all duration-300 hover:shadow-lg hover:shadow-brand-500/[0.03]`}
            >
              <div className="flex items-start justify-between">
                <div className={`inline-flex rounded-xl ${s.bg} p-2.5`}>
                  <s.icon className={`h-5 w-5 ${s.accent}`} />
                </div>
                <span className="font-mono text-xs text-neutral-600">{s.n}</span>
              </div>
              <h3 className="mt-4 font-display text-lg font-semibold text-white">{s.title}</h3>
              <code className={`mt-1 block font-mono text-[11px] ${s.accent}`}>{s.cmd}</code>
              <p className="mt-2.5 text-sm leading-relaxed text-neutral-400">{s.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
