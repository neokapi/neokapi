import { useState, type ReactNode } from "react";
import clsx from "clsx";
import Heading from "@theme/Heading";
import { Check, Copy, Terminal, Monitor, Download, GitBranch } from "lucide-react";
import styles from "./home.module.css";

// Install + first-run, folded in from the landing. Three surfaces: CLI (with
// install methods), Desktop, and a GitHub Actions gate. All commands are real.
const CLI_METHODS = [
  { label: "Homebrew", command: "brew install neokapi/tap/kapi" },
  { label: "WinGet", command: "winget install Neokapi.Kapi" },
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

const DOWNLOADS = [
  { os: "macOS", arch: "Apple Silicon", file: "kapi-desktop-macOS-arm64.dmg" },
  { os: "Windows", arch: "amd64 · arm64", file: "kapi-desktop-windows-setup.exe" },
  { os: "Linux", arch: "amd64 · arm64", file: "kapi-desktop-linux.tar.gz" },
];

type Tab = "cli" | "desktop" | "actions";

function Terminal_({
  label,
  copyId,
  copied,
  onCopy,
  children,
}: {
  label: string;
  copyId: string;
  copied: string | null;
  onCopy: (id: string) => void;
  children: ReactNode;
}) {
  return (
    <div className={styles.terminal}>
      <div className={styles.terminalBar}>
        <span className={styles.dots}>
          <span className={styles.dot} />
          <span className={styles.dot} />
          <span className={styles.dot} />
          <span className={styles.terminalLabel}>{label}</span>
        </span>
        <button
          type="button"
          className={styles.copyBtn}
          onClick={() => onCopy(copyId)}
          aria-label={`Copy ${label}`}
        >
          {copied === copyId ? <Check size={14} /> : <Copy size={14} />}
          {copied === copyId ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className={styles.terminalBody}>{children}</pre>
    </div>
  );
}

export default function GetStarted() {
  const [tab, setTab] = useState<Tab>("cli");
  const [method, setMethod] = useState(0);
  const [copied, setCopied] = useState<string | null>(null);

  function copyText(text: string, id: string) {
    if (typeof navigator !== "undefined" && navigator.clipboard) {
      navigator.clipboard.writeText(text);
    }
    setCopied(id);
    setTimeout(() => setCopied(null), 2000);
  }

  const tabs: { id: Tab; label: string; icon: typeof Terminal }[] = [
    { id: "cli", label: "Kapi CLI", icon: Terminal },
    { id: "desktop", label: "Kapi Desktop", icon: Monitor },
    { id: "actions", label: "GitHub Actions", icon: GitBranch },
  ];

  return (
    <section className={clsx(styles.section, styles.sectionAlt)} id="get-started">
      <div className="container">
        <div className={styles.sectionHead}>
          <Heading as="h2" className={styles.sectionTitle}>
            Install and run
          </Heading>
          <p className={styles.sectionLede}>
            One binary, offline by default. Use the CLI for automation and CI, the MCP server inside
            your AI assistant, or the desktop app for a visual workflow. No account required.
          </p>
        </div>

        <div className={styles.getStartedInner}>
          <div className={styles.tabRow}>
            {tabs.map((t) => (
              <button
                key={t.id}
                type="button"
                className={clsx(styles.tab, tab === t.id && styles.tabActive)}
                onClick={() => setTab(t.id)}
              >
                <t.icon size={16} aria-hidden="true" />
                {t.label}
              </button>
            ))}
          </div>

          {tab === "cli" && (
            <div>
              <div className={styles.subTabRow}>
                {CLI_METHODS.map((m, i) => (
                  <button
                    key={m.label}
                    type="button"
                    className={clsx(styles.subTab, i === method && styles.subTabActive)}
                    onClick={() => setMethod(i)}
                  >
                    {m.label}
                  </button>
                ))}
              </div>

              <Terminal_ label="install" copyId="install" copied={copied} onCopy={() => copyText(CLI_METHODS[method].command, "install")}>
                <span className={styles.prompt}>$ </span>
                {CLI_METHODS[method].command}
              </Terminal_>

              <Terminal_ label="quick start" copyId="quickstart" copied={copied} onCopy={() => copyText(QUICK_START, "quickstart")}>
                {QUICK_START}
              </Terminal_>
            </div>
          )}

          {tab === "desktop" && (
            <div>
              <Terminal_ label="install via Homebrew" copyId="cask" copied={copied} onCopy={() => copyText("brew install --cask neokapi/tap/kapi", "cask")}>
                <span className={styles.prompt}>$ </span>
                brew install --cask neokapi/tap/kapi
              </Terminal_>

              <div className={styles.downloadGrid}>
                {DOWNLOADS.map((dl) => (
                  <a
                    key={dl.os}
                    className={styles.download}
                    href="https://github.com/neokapi/neokapi/releases"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Download size={22} aria-hidden="true" className={styles.accent} />
                    <span className={styles.downloadOs}>{dl.os}</span>
                    <span className={styles.downloadArch}>{dl.arch}</span>
                    <code className={styles.downloadFile}>{dl.file}</code>
                  </a>
                ))}
              </div>

              <div className={styles.sectionFoot}>
                <a
                  className={styles.ctaLink}
                  href="https://github.com/neokapi/neokapi/releases"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  All releases on GitHub &rarr;
                </a>
              </div>
            </div>
          )}

          {tab === "actions" && (
            <div>
              <Terminal_ label=".github/workflows/translate.yml" copyId="actions" copied={copied} onCopy={() => copyText(ACTIONS_YAML, "actions")}>
                {ACTIONS_YAML}
              </Terminal_>
              <div className={styles.sectionFoot}>
                <a
                  className={styles.ctaLink}
                  href="https://github.com/neokapi/kapi-action"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  kapi-action on GitHub &rarr;
                </a>
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
