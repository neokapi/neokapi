import { useState } from "react";
import { Check, Copy, Terminal, Monitor, Download, GitBranch } from "lucide-react";
import { cn } from "@/lib/utils";

const CLI_METHODS = [
  {
    label: "Homebrew",
    command: "brew install neokapi/tap/kapi",
  },
  {
    label: "WinGet",
    command: "winget install Neokapi.Kapi",
  },
  {
    label: "Binary",
    command:
      "curl -sSL https://github.com/neokapi/neokapi/releases/latest/download/kapi_darwin_arm64.tar.gz | tar xz",
  },
];

const QUICK_START = `# Extract the text from any file into blocks
kapi extract quarterly-report.docx -o strings.json

# Write the changed text back into the original file, faithfully
kapi merge strings.de.json --skeleton quarterly-report.docx \\
  -o quarterly-report.de.docx

# Translate and run QA in one flow
kapi run ai-translate-qa -i app.json -o app.de.json \\
  --source-lang en --target-lang de

# Score content against a brand profile; --min-score gates CI (exit 3)
kapi brand check --profile-file brand.yaml --min-score 80 release-notes.md

# Serve the engine to your AI assistant over MCP
kapi mcp`;

const ACTIONS_YAML = `name: Localization checks

on:
  pull_request:
    paths:
      - 'content/**'
      - 'src/locales/**'

jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: neokapi/kapi-action@v1
        with:
          # Fails the PR on any placeholder, terminology, or brand finding
          command: verify --target-lang de`;

export function GetStarted() {
  const [tab, setTab] = useState<"cli" | "desktop" | "actions">("cli");
  const [method, setMethod] = useState(0);
  const [copied, setCopied] = useState<string | null>(null);

  function copyText(text: string, id: string) {
    navigator.clipboard.writeText(text);
    setCopied(id);
    setTimeout(() => setCopied(null), 2000);
  }

  return (
    <section id="get-started" className="relative px-6 py-24">
      <div className="mx-auto max-w-4xl">
        <div className="mb-12 text-center">
          <h2 className="font-display text-3xl font-bold tracking-tight text-white sm:text-4xl">
            Up and running in{" "}
            <span className="bg-gradient-to-r from-brand-400 to-brand-300 bg-clip-text text-transparent">
              30 seconds
            </span>
          </h2>
          <p className="mx-auto mt-4 max-w-xl text-lg text-neutral-400">
            One binary, offline by default. Use the CLI for automation and CI, the MCP server inside
            your AI assistant, or the Desktop app for a visual workflow. No account required.
          </p>
        </div>

        {/* CLI / Desktop toggle */}
        <div className="mb-8 flex items-center justify-center gap-2">
          <button
            onClick={() => setTab("cli")}
            className={cn(
              "flex items-center gap-2 rounded-xl px-5 py-2.5 font-display text-sm font-medium transition-all",
              tab === "cli"
                ? "bg-brand-500/10 text-brand-400 border border-brand-500/20"
                : "text-neutral-500 hover:text-neutral-300 border border-transparent",
            )}
          >
            <Terminal className="h-4 w-4" />
            Kapi CLI
          </button>
          <button
            onClick={() => setTab("desktop")}
            className={cn(
              "flex items-center gap-2 rounded-xl px-5 py-2.5 font-display text-sm font-medium transition-all",
              tab === "desktop"
                ? "bg-brand-500/10 text-brand-400 border border-brand-500/20"
                : "text-neutral-500 hover:text-neutral-300 border border-transparent",
            )}
          >
            <Monitor className="h-4 w-4" />
            Kapi Desktop
          </button>
          <button
            onClick={() => setTab("actions")}
            className={cn(
              "flex items-center gap-2 rounded-xl px-5 py-2.5 font-display text-sm font-medium transition-all",
              tab === "actions"
                ? "bg-brand-500/10 text-brand-400 border border-brand-500/20"
                : "text-neutral-500 hover:text-neutral-300 border border-transparent",
            )}
          >
            <GitBranch className="h-4 w-4" />
            GitHub Actions
          </button>
        </div>

        {tab === "cli" && (
          <div>
            {/* Install method tabs */}
            <div className="mb-6 flex items-center gap-2">
              {CLI_METHODS.map((m, i) => (
                <button
                  key={m.label}
                  onClick={() => setMethod(i)}
                  className={cn(
                    "rounded-lg px-4 py-2 font-display text-sm font-medium transition-all",
                    i === method
                      ? "bg-brand-500/10 text-brand-400 border border-brand-500/20"
                      : "text-neutral-500 hover:text-neutral-300 border border-transparent",
                  )}
                >
                  {m.label}
                </button>
              ))}
            </div>

            {/* Install command */}
            <div className="terminal-window rounded-xl overflow-hidden">
              <div className="flex items-center justify-between border-b border-brand-500/8 px-5 py-3">
                <div className="flex items-center gap-2">
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
                  <span className="ml-3 font-mono text-xs text-neutral-600">install</span>
                </div>
                <button
                  onClick={() => copyText(CLI_METHODS[method].command, "install")}
                  className="flex items-center gap-1.5 text-xs text-neutral-500 transition hover:text-brand-400"
                >
                  {copied === "install" ? (
                    <Check className="h-3.5 w-3.5" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                  {copied === "install" ? "Copied" : "Copy"}
                </button>
              </div>
              <div className="p-5">
                <pre className="font-mono text-sm">
                  <span className="select-none text-brand-400">$ </span>
                  <span className="text-neutral-200">{CLI_METHODS[method].command}</span>
                </pre>
              </div>
            </div>

            {/* Quick start */}
            <div className="mt-8 terminal-window rounded-xl overflow-hidden">
              <div className="flex items-center justify-between border-b border-brand-500/8 px-5 py-3">
                <div className="flex items-center gap-2">
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
                  <span className="ml-3 font-mono text-xs text-neutral-600">quick start</span>
                </div>
                <button
                  onClick={() => copyText(QUICK_START, "quickstart")}
                  className="flex items-center gap-1.5 text-xs text-neutral-500 transition hover:text-brand-400"
                >
                  {copied === "quickstart" ? (
                    <Check className="h-3.5 w-3.5" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                  {copied === "quickstart" ? "Copied" : "Copy"}
                </button>
              </div>
              <div className="p-5">
                <pre className="font-mono text-sm leading-relaxed text-neutral-300 whitespace-pre-wrap">
                  {QUICK_START}
                </pre>
              </div>
            </div>
          </div>
        )}
        {tab === "desktop" && (
          <div>
            {/* Homebrew cask */}
            <div className="mb-8 terminal-window rounded-xl overflow-hidden">
              <div className="flex items-center justify-between border-b border-brand-500/8 px-5 py-3">
                <div className="flex items-center gap-2">
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
                  <span className="ml-3 font-mono text-xs text-neutral-600">
                    install via Homebrew
                  </span>
                </div>
                <button
                  onClick={() => copyText("brew install --cask neokapi/tap/kapi", "cask")}
                  className="flex items-center gap-1.5 text-xs text-neutral-500 transition hover:text-brand-400"
                >
                  {copied === "cask" ? (
                    <Check className="h-3.5 w-3.5" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                  {copied === "cask" ? "Copied" : "Copy"}
                </button>
              </div>
              <div className="p-5">
                <pre className="font-mono text-sm">
                  <span className="select-none text-brand-400">$ </span>
                  <span className="text-neutral-200">brew install --cask neokapi/tap/kapi</span>
                </pre>
              </div>
            </div>

            {/* Desktop download */}
            <div className="rounded-2xl border border-surface-700/50 bg-surface-900/40 p-8">
              <div className="text-center mb-8">
                <p className="text-neutral-400">
                  Or download directly for Windows, macOS, or Linux. Same tools, same flows, same
                  project files as the CLI.
                </p>
              </div>

              <div className="grid gap-4 sm:grid-cols-3">
                {[
                  { os: "macOS", arch: "Apple Silicon", file: "kapi-desktop-macOS-arm64.dmg" },
                  { os: "Windows", arch: "amd64 · arm64", file: "kapi-desktop-windows-setup.exe" },
                  { os: "Linux", arch: "amd64 · arm64", file: "kapi-desktop-linux.tar.gz" },
                ].map((dl) => (
                  <a
                    key={dl.os}
                    href="https://github.com/neokapi/neokapi/releases"
                    target="_blank"
                    rel="noopener"
                    className="group flex flex-col items-center gap-3 rounded-xl border border-surface-700/50 bg-surface-800/40 px-4 py-5 transition-all hover:border-brand-500/20 hover:bg-surface-800/60"
                  >
                    <Download className="h-6 w-6 text-brand-400 transition-transform group-hover:-translate-y-0.5" />
                    <div className="text-center">
                      <div className="font-display text-sm font-semibold text-neutral-200">
                        {dl.os}
                      </div>
                      <div className="text-xs text-neutral-500">{dl.arch}</div>
                    </div>
                    <code className="text-[10px] text-neutral-600">{dl.file}</code>
                  </a>
                ))}
              </div>

              <div className="mt-8 text-center">
                <a
                  href="https://github.com/neokapi/neokapi/releases"
                  target="_blank"
                  rel="noopener"
                  className="inline-flex items-center gap-2 text-sm text-brand-400 transition hover:text-brand-300"
                >
                  All releases on GitHub
                  <span className="text-xs">&rarr;</span>
                </a>
              </div>
            </div>
          </div>
        )}
        {tab === "actions" && (
          <div>
            {/* GitHub Actions */}
            <div className="terminal-window rounded-xl overflow-hidden">
              <div className="flex items-center justify-between border-b border-brand-500/8 px-5 py-3">
                <div className="flex items-center gap-2">
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-rose/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-accent-amber/50" />
                  <div className="h-2.5 w-2.5 rounded-full bg-brand-500/50" />
                  <span className="ml-3 font-mono text-xs text-neutral-600">
                    .github/workflows/translate.yml
                  </span>
                </div>
                <button
                  onClick={() => copyText(ACTIONS_YAML, "actions")}
                  className="flex items-center gap-1.5 text-xs text-neutral-500 transition hover:text-brand-400"
                >
                  {copied === "actions" ? (
                    <Check className="h-3.5 w-3.5" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                  {copied === "actions" ? "Copied" : "Copy"}
                </button>
              </div>
              <div className="p-5">
                <pre className="font-mono text-sm leading-relaxed text-neutral-300 whitespace-pre-wrap">
                  {ACTIONS_YAML}
                </pre>
              </div>
            </div>

            <div className="mt-6 text-center">
              <a
                href="https://github.com/neokapi/kapi-action"
                target="_blank"
                rel="noopener"
                className="inline-flex items-center gap-2 text-sm text-brand-400 transition hover:text-brand-300"
              >
                kapi-action on GitHub
                <span className="text-xs">&rarr;</span>
              </a>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
