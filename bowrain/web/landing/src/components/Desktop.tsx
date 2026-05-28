import { Apple, Monitor, Download } from "lucide-react";

export function Desktop() {
  const base = import.meta.env.BASE_URL;

  return (
    <section id="desktop" className="mx-auto max-w-6xl px-6 py-24">
      <div className="mx-auto max-w-3xl text-center">
        <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-neutral-800 px-3 py-1 font-mono text-xs text-neutral-400">
          DESKTOP APP
        </div>
        <h2 className="text-2xl font-bold tracking-tight text-white sm:text-3xl">
          A native app, with a visual flow builder
        </h2>
        <p className="mt-3 text-neutral-400">
          Bowrain Desktop signs in to the same workspace as the web editor — same projects, memory,
          and terminology — and adds a drag-and-drop flow builder for composing translation
          pipelines. Works offline; changes sync when you reconnect.
        </p>
      </div>

      {/* Real desktop screenshot, captured from the live app. */}
      <div className="mt-12 overflow-hidden rounded-xl border border-neutral-800 bg-neutral-900/50 shadow-2xl shadow-brand-500/5">
        <div className="flex items-center gap-2 border-b border-neutral-800 bg-neutral-900/80 px-4 py-2.5">
          <div className="h-2.5 w-2.5 rounded-full bg-red-500/60" />
          <div className="h-2.5 w-2.5 rounded-full bg-yellow-500/60" />
          <div className="h-2.5 w-2.5 rounded-full bg-green-500/60" />
          <span className="ml-2 text-xs text-neutral-500">Bowrain Desktop — Flows</span>
        </div>
        <img
          src={`${base}desktop-flows.png`}
          alt="The Bowrain Desktop flow builder: a visual node graph wiring Input → AI Translate → QA Check → Output, with a library of built-in flows."
          className="block w-full"
        />
      </div>

      {/* Download links */}
      <div className="mt-8 flex flex-wrap items-center justify-center gap-4">
        <a
          href="#download-mac"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 bg-neutral-900/50 px-5 py-2.5 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <Apple className="h-4 w-4" />
          macOS
        </a>
        <a
          href="#download-windows"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 bg-neutral-900/50 px-5 py-2.5 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <Monitor className="h-4 w-4" />
          Windows
        </a>
        <a
          href="#download-linux"
          className="flex items-center gap-2 rounded-lg border border-neutral-700 bg-neutral-900/50 px-5 py-2.5 text-sm text-neutral-300 transition hover:border-neutral-500 hover:text-white"
        >
          <Download className="h-4 w-4" />
          Linux
        </a>
      </div>
      <p className="mt-3 text-center text-xs text-neutral-600">
        Works offline. Changes sync automatically when you reconnect.
      </p>
    </section>
  );
}
