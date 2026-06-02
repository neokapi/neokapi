import { Plug, Zap, PenTool, GitBranch } from "lucide-react";

export function Platform() {
  return (
    <section id="platform" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-neutral-800 px-3 py-1 text-xs text-neutral-400 font-mono">
          TEAM PLATFORM
        </div>
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          <span className="text-brand-400">Bowrain</span> — the layer a team hosts and governs
        </h2>
        <p className="mt-3 text-neutral-400">
          Multi-user workspaces with authentication, server-side content storage and version
          history, connectors, automation, and a collaborative editor — the durable layer on top of
          the kapi toolchain your team already uses locally.
        </p>
        <div className="mt-4 inline-block overflow-hidden rounded-lg border border-neutral-800 bg-neutral-950 px-4 py-2 font-mono text-sm">
          <span className="text-suggestion">$</span>{" "}
          <span className="text-neutral-400">brew install neokapi/tap/bowrain-cli</span>
        </div>
      </div>

      <div className="mt-12 grid gap-6 md:grid-cols-2">
        {/* Live Connectors */}
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
              <Plug className="h-5 w-5 text-brand-400" />
            </div>
            <h3 className="text-lg font-semibold text-white">Live connectors</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            Bidirectional sync with your content systems. Content flows in, translations flow back.
            No export-import cycle.
          </p>
          <div className="mt-4 grid grid-cols-2 gap-2 text-xs">
            {[
              { label: "CMS", examples: "Contentful, Strapi, WordPress" },
              { label: "Design", examples: "Figma, Sketch" },
              { label: "Code", examples: "Git repos, CI/CD" },
              { label: "Files", examples: "kapi push / pull" },
            ].map((c) => (
              <div key={c.label} className="rounded-lg bg-neutral-800/50 p-3">
                <div className="font-medium text-neutral-300">{c.label}</div>
                <div className="mt-0.5 text-neutral-500">{c.examples}</div>
              </div>
            ))}
          </div>
        </div>

        {/* Event-Driven Automation */}
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
              <Zap className="h-5 w-5 text-brand-400" />
            </div>
            <h3 className="text-lg font-semibold text-white">Automated workflows</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            Set up rules that react automatically. New content arrives — translation starts. Quality
            checks pass — your team gets notified. Bad translations get caught before they ship.
          </p>
          <div className="mt-4 overflow-hidden rounded-lg border border-neutral-800 bg-neutral-950 p-4 font-mono text-xs text-neutral-400">
            <div className="text-neutral-600"># Automation rule</div>
            <div>
              <span className="text-brand-400">on</span>: content.pushed
            </div>
            <div>
              <span className="text-brand-400">run</span>:
            </div>
            <div>
              {" "}
              - <span className="text-neutral-300">ai-translate</span>{" "}
              <span className="text-neutral-600"># translate new blocks</span>
            </div>
            <div>
              {" "}
              - <span className="text-neutral-300">qa-check</span>{" "}
              <span className="text-neutral-600"># run quality checks</span>
            </div>
            <div>
              {" "}
              - <span className="text-neutral-300">notify</span>{" "}
              <span className="text-neutral-600"># webhook on completion</span>
            </div>
          </div>
        </div>

        {/* Visual Editor */}
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
              <PenTool className="h-5 w-5 text-brand-400" />
            </div>
            <h3 className="text-lg font-semibold text-white">Visual translation editor</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            Web and desktop editors for human review. Side-by-side preview, translation suggestions,
            glossary highlights, and real-time collaboration — see who's editing what.
          </p>
          <div className="mt-4 grid grid-cols-2 gap-2 text-xs">
            {[
              "Grid / Focus / Split layouts",
              "Translation suggestions",
              "Glossary highlights",
              "See who\u2019s editing",
              "Offline-first desktop app",
              "Progress tracking",
            ].map((f) => (
              <div key={f} className="flex items-center gap-2 text-neutral-400">
                <span className="text-brand-400">·</span>
                {f}
              </div>
            ))}
          </div>
        </div>

        {/* Git-Like Workflow */}
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
              <GitBranch className="h-5 w-5 text-brand-400" />
            </div>
            <h3 className="text-lg font-semibold text-white">Git-like workflow</h3>
          </div>
          <p className="text-sm leading-relaxed text-neutral-400">
            kapi manages a{" "}
            <code className="rounded bg-neutral-800 px-1 text-xs text-neutral-300">.kapi/</code>{" "}
            project directory and syncs it with Bowrain, the way git pushes to and pulls from a
            hosted repository. Only changed content syncs.
          </p>
          <div className="mt-4 overflow-hidden rounded-lg border border-neutral-800 bg-neutral-950 p-4 font-mono text-xs">
            <div>
              <span className="text-suggestion">$</span>{" "}
              <span className="text-neutral-400">kapi init</span>
            </div>
            <div>
              <span className="text-suggestion">$</span>{" "}
              <span className="text-neutral-400">kapi push -m "v2.1 strings"</span>
            </div>
            <div className="text-neutral-600"> → 47 blocks pushed (12 new, 8 changed)</div>
            <div className="mt-2">
              <span className="text-suggestion">$</span>{" "}
              <span className="text-neutral-400">kapi pull</span>
            </div>
            <div className="text-neutral-600"> → 47 blocks pulled for de, fr, ja</div>
            <div className="mt-2">
              <span className="text-suggestion">$</span>{" "}
              <span className="text-neutral-400">kapi status</span>
            </div>
            <div className="text-neutral-600"> → de: 47/47 ✓ fr: 45/47 ja: 41/47</div>
          </div>
        </div>
      </div>
    </section>
  );
}
