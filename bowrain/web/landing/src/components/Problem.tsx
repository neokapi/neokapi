import { Package, Pen, Layers } from "lucide-react";

export function Problem() {
  return (
    <section className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          Where the local toolchain ends
        </h2>
        <p className="mt-3 text-neutral-400">
          kapi gives one person everything they need on their own machine. Bowrain begins where a
          team does — when the same brand voice, terminology, and translation memory have to be
          shared, versioned, and stewarded over time.
        </p>
      </div>

      <div className="mt-12 grid gap-6 md:grid-cols-3">
        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-brand-500/10">
            <Package className="h-5 w-5 text-brand-400" />
          </div>
          <h3 className="text-lg font-semibold text-white">Shared, not scattered</h3>
          <p className="mt-2 text-sm leading-relaxed text-neutral-400">
            One brand voice, terminology base, and translation memory for everyone — not a copy on
            each laptop that drifts out of sync.
          </p>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-suggestion/10">
            <Pen className="h-5 w-5 text-suggestion" />
          </div>
          <h3 className="text-lg font-semibold text-white">Versioned and audited</h3>
          <p className="mt-2 text-sm leading-relaxed text-neutral-400">
            Version history, audit, and review for content and translations, so a team can see what
            changed, who changed it, and roll back when needed.
          </p>
        </div>

        <div className="rounded-xl border border-neutral-800 bg-neutral-900/30 p-6">
          <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-lg bg-preserved/10">
            <Layers className="h-5 w-5 text-preserved" />
          </div>
          <h3 className="text-lg font-semibold text-white">Connected to your systems</h3>
          <p className="mt-2 text-sm leading-relaxed text-neutral-400">
            Connectors to the systems content already lives in, and automation that runs on the
            server — so the work keeps moving without anyone running a command.
          </p>
        </div>
      </div>
    </section>
  );
}
