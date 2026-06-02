import { useState } from "react";
import { cn } from "@/lib/utils";
import { Monitor, FolderOpen, Play, Wrench, BookOpen, Settings } from "lucide-react";

const TABS = [
  {
    label: "Projects & Ad-hoc",
    icon: FolderOpen,
    description:
      "Work with projects for organized, repeatable workflows, or go ad-hoc on individual files. Same project config works across Desktop and CLI.",
    screen: ProjectsScreen,
  },
  {
    label: "Tools & Flows",
    icon: Play,
    description:
      "Execute individual tools like ai-translate, pseudo-translate, or word-count. Compose them into flows and run with one click.",
    screen: ToolsScreen,
  },
  {
    label: "Formats & Settings",
    icon: Wrench,
    description:
      "Configure and tweak format readers and writers. Adjust parameters, encoding, segmentation rules, and output options.",
    screen: FormatsScreen,
  },
  {
    label: "TM & Termbases",
    icon: BookOpen,
    description:
      "Manage translation memories and terminology databases. Import, export, browse entries, and configure matching thresholds.",
    screen: TmScreen,
  },
];

function ProjectsScreen() {
  const projects = [
    { name: "mobile-app", langs: "en → fr, de, ja", files: 24, status: "active" },
    { name: "docs-site", langs: "en → es, pt, zh", files: 67, status: "active" },
    { name: "marketing", langs: "en → fr, de", files: 8, status: "idle" },
  ];
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between px-1 pb-2">
        <span className="text-xs font-semibold text-neutral-400 uppercase tracking-wider">
          Recent Projects
        </span>
        <span className="rounded bg-brand-500/15 px-2 py-0.5 text-[10px] font-medium text-brand-400">
          + New
        </span>
      </div>
      {projects.map((p) => (
        <div
          key={p.name}
          className="flex items-center justify-between rounded-lg bg-surface-800/60 px-3 py-2.5 border border-surface-700/30"
        >
          <div className="flex items-center gap-3">
            <FolderOpen className="h-4 w-4 text-brand-400" />
            <div>
              <div className="text-sm font-medium text-neutral-200">{p.name}</div>
              <div className="text-[11px] text-neutral-500">{p.langs}</div>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-[11px] text-neutral-600">{p.files} files</span>
            <span
              className={cn(
                "rounded-full px-2 py-0.5 text-[10px] font-medium",
                p.status === "active"
                  ? "bg-brand-500/15 text-brand-400"
                  : "bg-surface-600/30 text-neutral-500",
              )}
            >
              {p.status}
            </span>
          </div>
        </div>
      ))}
      <div className="mt-2 rounded-lg border border-dashed border-surface-600 bg-surface-800/20 px-3 py-2.5">
        <div className="flex items-center gap-2 text-xs text-neutral-400">
          <Settings className="h-3.5 w-3.5 text-neutral-500" />
          <span>
            Or drag files here for <span className="text-brand-400">ad-hoc</span> processing
          </span>
        </div>
      </div>
      <div className="mt-1 rounded-lg border border-dashed border-surface-600 px-3 py-2 text-center">
        <span className="text-xs text-neutral-500">Shared project via the </span>
        <code className="text-xs text-brand-400">.kapi</code>
        <span className="text-xs text-neutral-500"> recipe — works in Desktop & CLI</span>
      </div>
    </div>
  );
}

function ToolsScreen() {
  const tools = [
    { name: "ai-translate", desc: "LLM translation", status: "ready", accent: "text-accent-cyan" },
    { name: "pseudo-translate", desc: "i18n testing", status: "ready", accent: "text-brand-400" },
    { name: "word-count", desc: "Cost estimation", status: "ready", accent: "text-brand-400" },
    { name: "qa-check", desc: "Quality validation", status: "ready", accent: "text-forest-400" },
  ];
  const flows = [
    { name: "translate", tools: "terminology → ai-translate → qa-check", status: "run" },
    { name: "review", tools: "tm-lookup → qa-check", status: "run" },
  ];
  return (
    <div className="space-y-3">
      <div>
        <span className="text-xs font-semibold text-neutral-400 uppercase tracking-wider px-1">
          Tools
        </span>
        <div className="mt-2 space-y-1.5">
          {tools.map((t) => (
            <div
              key={t.name}
              className="flex items-center justify-between rounded-lg bg-surface-800/40 border border-surface-700/30 px-3 py-2"
            >
              <div className="flex items-center gap-2">
                <code className={cn("text-xs font-medium", t.accent)}>{t.name}</code>
                <span className="text-[11px] text-neutral-600">{t.desc}</span>
              </div>
              <button className="rounded bg-surface-700/50 px-2 py-0.5 text-[10px] text-neutral-400 hover:bg-surface-600/50 hover:text-neutral-300 transition">
                run
              </button>
            </div>
          ))}
        </div>
      </div>
      <div>
        <span className="text-xs font-semibold text-neutral-400 uppercase tracking-wider px-1">
          Flows
        </span>
        <div className="mt-2 space-y-1.5">
          {flows.map((f) => (
            <div
              key={f.name}
              className="flex items-center justify-between rounded-lg bg-surface-800/40 border border-surface-700/30 px-3 py-2"
            >
              <div>
                <code className="text-xs font-medium text-brand-300">{f.name}</code>
                <div className="text-[10px] text-neutral-600 mt-0.5">{f.tools}</div>
              </div>
              <button className="rounded bg-brand-500/15 px-2 py-0.5 text-[10px] font-medium text-brand-400 hover:bg-brand-500/25 transition">
                <Play className="inline h-2.5 w-2.5 mr-0.5" />
                run
              </button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function FormatsScreen() {
  const formats = [
    { name: "JSON", params: "key-value nested", active: true },
    { name: "HTML", params: "inline tags, attrs", active: true },
    { name: "Markdown", params: "front-matter, code", active: false },
    { name: "DOCX", params: "runs, styles", active: false },
  ];
  return (
    <div className="space-y-2">
      <span className="text-xs font-semibold text-neutral-400 uppercase tracking-wider px-1">
        Format Configuration
      </span>
      <div className="mt-2 space-y-1.5">
        {formats.map((f) => (
          <div
            key={f.name}
            className="rounded-lg bg-surface-800/40 border border-surface-700/30 px-3 py-2.5"
          >
            <div className="flex items-center justify-between">
              <code className="text-xs font-medium text-neutral-200">{f.name}</code>
              <div
                className={cn(
                  "h-3 w-6 rounded-full transition-colors relative",
                  f.active ? "bg-brand-500/40" : "bg-surface-600/50",
                )}
              >
                <div
                  className={cn(
                    "absolute top-0.5 h-2 w-2 rounded-full transition-all",
                    f.active ? "right-0.5 bg-brand-400" : "left-0.5 bg-neutral-500",
                  )}
                />
              </div>
            </div>
            <div className="text-[10px] text-neutral-600 mt-1">{f.params}</div>
          </div>
        ))}
      </div>
      <div className="rounded-lg bg-surface-800/40 border border-surface-700/30 px-3 py-2">
        <div className="flex items-center justify-between">
          <span className="text-[11px] text-neutral-400">Output encoding</span>
          <code className="text-[11px] text-brand-400">UTF-8</code>
        </div>
        <div className="flex items-center justify-between mt-1.5">
          <span className="text-[11px] text-neutral-400">Segmentation</span>
          <code className="text-[11px] text-brand-400">SRX default</code>
        </div>
      </div>
    </div>
  );
}

function TmScreen() {
  const tms = [
    { name: "project-tm", entries: "2,847", langs: "en → fr, de, ja", type: "TM" },
    { name: "glossary", entries: "342", langs: "en → fr, de", type: "TB" },
  ];
  const recentEntries = [
    { src: "Cancel subscription", tgt: "Annuler l'abonnement", score: "100%" },
    { src: "Payment method", tgt: "Moyen de paiement", score: "100%" },
    { src: "Account settings", tgt: "Paramètres du compte", score: "92%" },
  ];
  return (
    <div className="space-y-3">
      <div>
        <div className="flex items-center justify-between px-1 pb-1">
          <span className="text-xs font-semibold text-neutral-400 uppercase tracking-wider">
            Resources
          </span>
          <span className="rounded bg-brand-500/15 px-2 py-0.5 text-[10px] font-medium text-brand-400">
            + Import
          </span>
        </div>
        <div className="space-y-1.5 mt-1">
          {tms.map((tm) => (
            <div
              key={tm.name}
              className="flex items-center justify-between rounded-lg bg-surface-800/40 border border-surface-700/30 px-3 py-2"
            >
              <div className="flex items-center gap-2">
                <span
                  className={cn(
                    "rounded px-1.5 py-0.5 text-[9px] font-bold",
                    tm.type === "TM"
                      ? "bg-forest-500/15 text-forest-400"
                      : "bg-accent-amber/15 text-accent-amber",
                  )}
                >
                  {tm.type}
                </span>
                <div>
                  <code className="text-xs text-neutral-200">{tm.name}</code>
                  <div className="text-[10px] text-neutral-600">
                    {tm.entries} entries &middot; {tm.langs}
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
      <div>
        <span className="text-xs font-semibold text-neutral-400 uppercase tracking-wider px-1">
          Recent Matches
        </span>
        <div className="space-y-1 mt-2">
          {recentEntries.map((e, i) => (
            <div
              key={i}
              className="grid grid-cols-[1fr_1fr_auto] gap-2 rounded-lg bg-surface-800/40 border border-surface-700/30 px-3 py-1.5"
            >
              <span className="text-[11px] text-neutral-400 truncate">{e.src}</span>
              <span className="text-[11px] text-neutral-300 truncate">{e.tgt}</span>
              <span className="text-[10px] text-forest-400 font-medium">{e.score}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

export function DesktopDemo() {
  const [active, setActive] = useState(0);
  const ActiveScreen = TABS[active].screen;

  return (
    <section id="desktop" className="relative px-6 py-24">
      <div className="mx-auto max-w-6xl">
        <div className="mb-16 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            The{" "}
            <span className="bg-gradient-to-r from-brand-400 to-forest-400 bg-clip-text text-transparent">
              Kapi Desktop
            </span>{" "}
            app
          </h2>
          <p className="mx-auto mt-4 max-w-2xl text-lg text-neutral-400">
            Everything the CLI does, with a visual interface. Work ad-hoc or with projects &mdash;
            shared across Desktop and CLI.
          </p>
        </div>

        <div className="grid gap-8 lg:grid-cols-[1.4fr_1fr]">
          {/* App window (left on desktop — mirrored from CLI layout) */}
          <div className="overflow-hidden rounded-2xl border border-surface-700/50 bg-surface-900/60 shadow-2xl glow-teal order-2 lg:order-1">
            {/* Title bar */}
            <div className="flex items-center justify-between border-b border-surface-700/30 px-4 py-2.5">
              <div className="flex items-center gap-2">
                <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
                <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
                <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
              </div>
              <div className="flex items-center gap-1.5">
                <Monitor className="h-3 w-3 text-neutral-600" />
                <span className="font-display text-xs text-neutral-500 font-medium">
                  Kapi Desktop
                </span>
              </div>
              <div className="w-16" />
            </div>

            {/* Sidebar + content */}
            <div className="flex min-h-[340px]">
              {/* Mini sidebar */}
              <div className="flex w-12 flex-col items-center gap-1 border-r border-surface-700/30 bg-surface-950/40 py-3">
                {TABS.map((tab, i) => (
                  <button
                    key={tab.label}
                    onClick={() => setActive(i)}
                    className={cn(
                      "flex h-8 w-8 items-center justify-center rounded-lg transition-all",
                      i === active
                        ? "bg-brand-500/15 text-brand-400"
                        : "text-neutral-600 hover:text-neutral-400 hover:bg-surface-800/50",
                    )}
                    title={tab.label}
                  >
                    <tab.icon className="h-4 w-4" />
                  </button>
                ))}
              </div>

              {/* Main content */}
              <div className="flex-1 p-4 overflow-y-auto">
                <ActiveScreen />
              </div>
            </div>

            {/* Status bar */}
            <div className="flex items-center justify-between border-t border-surface-700/30 px-4 py-1.5">
              <span className="text-[10px] text-neutral-600">mobile-app</span>
              <div className="flex items-center gap-3">
                <span className="text-[10px] text-neutral-600">Windows / macOS / Linux</span>
              </div>
            </div>
          </div>

          {/* Tab descriptions (right on desktop) */}
          <div className="flex flex-col gap-2 order-1 lg:order-2">
            {TABS.map((tab, i) => (
              <button
                key={tab.label}
                onClick={() => setActive(i)}
                className={cn(
                  "group rounded-xl px-5 py-4 text-left transition-all duration-200",
                  i === active
                    ? "border border-brand-500/20 bg-brand-500/[0.06] shadow-lg shadow-brand-500/[0.03]"
                    : "border border-transparent hover:border-surface-600 hover:bg-surface-800/50",
                )}
              >
                <div className="flex items-center gap-3">
                  <div
                    className={cn(
                      "h-2 w-2 rounded-full transition-colors",
                      i === active ? "bg-brand-400" : "bg-surface-600 group-hover:bg-surface-500",
                    )}
                  />
                  <span
                    className={cn(
                      "font-display text-sm font-semibold transition-colors",
                      i === active
                        ? "text-brand-300"
                        : "text-neutral-400 group-hover:text-neutral-300",
                    )}
                  >
                    {tab.label}
                  </span>
                </div>
                <p
                  className={cn(
                    "mt-2 ml-5 text-sm leading-relaxed transition-colors",
                    i === active ? "text-neutral-300" : "text-neutral-500",
                  )}
                >
                  {tab.description}
                </p>
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
