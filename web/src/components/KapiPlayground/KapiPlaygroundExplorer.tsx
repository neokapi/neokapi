import React from "react";
import { ChunkSafeSuspense } from "../ChunkErrorBoundary";
import { lazyWithRetry } from "../../lib/chunkReload";
import BrowserOnly from "@docusaurus/BrowserOnly";
import { FileText, FolderGit2, Play } from "lucide-react";
import { useKapiPlaygroundConfig } from "./config";
import { installBrowserTranslateBridge } from "../../lib/browserTranslate";
import { PlaygroundDialog } from "./PlaygroundDialog";
import "./explorer.css";

// The consolidated CLI playground: a full-bleed in-browser kapi terminal beside
// a sample picker. The picker lists two visually distinct groups —
//   • "Loose files (ad-hoc)" — a single standalone input you run a one-off
//     command against, and
//   • "Sample projects" — a ready-made kapi.yaml recipe + content + a seeded
//     content memory, so you can run the offline project funnel.
// Selecting a sample seeds the in-memory FS and stages a suggested command at
// the prompt. The heavy kit (xterm + wasm) is one async chunk, loaded once.

const LazyExplorer = lazyWithRetry(async () => {
  const { KapiEmbed, LOOSE_SAMPLES, PROJECT_SAMPLES } = await import("@neokapi/kapi-playground");
  type KapiEmbedHandle = import("@neokapi/kapi-playground").KapiEmbedHandle;
  type LooseSample = import("@neokapi/kapi-playground").LooseSample;
  type ProjectSample = import("@neokapi/kapi-playground").ProjectSample;

  function Explorer(): React.ReactElement {
    const cfg = useKapiPlaygroundConfig();
    const embedRef = React.useRef<KapiEmbedHandle>(null);
    const [activeId, setActiveId] = React.useState<string>(`loose:${LOOSE_SAMPLES[0].id}`);
    const [pickerOpen, setPickerOpen] = React.useState(
      () => !window.matchMedia("(max-width: 900px)").matches,
    );
    const pickerId = React.useId();

    // Install the on-device Translator bridge so `kapi exec mt-translate --provider
    // browser` works in the terminal where the browser supports it (free; no
    // model load until a translation runs).
    React.useEffect(() => {
      installBrowserTranslateBridge();
    }, []);

    const loadLoose = React.useCallback((s: LooseSample) => {
      setActiveId(`loose:${s.id}`);
      if (window.matchMedia("(max-width: 900px)").matches) setPickerOpen(false);
      embedRef.current?.openWith({
        files: [s.file],
        cmd: s.suggested,
        // Leave the suggested command at the prompt — the reader presses Enter.
        autoRun: false,
      });
    }, []);

    const loadProject = React.useCallback((s: ProjectSample) => {
      setActiveId(`project:${s.id}`);
      if (window.matchMedia("(max-width: 900px)").matches) setPickerOpen(false);
      embedRef.current?.openWith({
        files: s.files,
        binaryFiles:
          s.binary && s.contentBytes
            ? [{ path: s.contentName, bytes: s.contentBytes() }]
            : undefined,
        // Stage the funnel's first real step; the reader runs the rest.
        cmd: `kapi add ${s.contentName}`,
        autoRun: false,
      });
    }, []);

    return (
      <div className={`kapi-pgx${pickerOpen ? "" : " kapi-pgx--picker-closed"}`}>
        <div className="kapi-pgx__controls">
          <button
            type="button"
            className="button button--secondary button--sm"
            aria-expanded={pickerOpen}
            aria-controls={pickerId}
            onClick={() => setPickerOpen((value) => !value)}
          >
            {pickerOpen ? "Hide samples" : "Choose a sample"}
          </button>
          <span>
            Select a sample or upload files in the Files pane. Press Enter to run a command.
          </span>
        </div>
        <aside
          id={pickerId}
          className="kapi-pgx__picker"
          aria-label="Sample picker"
          hidden={!pickerOpen}
        >
          <section className="kapi-pgx__group kapi-pgx__group--loose">
            <h2 className="kapi-pgx__group-title">
              <FileText size={15} aria-hidden="true" />
              Loose files (ad-hoc)
            </h2>
            <p className="kapi-pgx__group-sub">One file. Run a single command against it.</p>
            <ul className="kapi-pgx__list">
              {LOOSE_SAMPLES.map((s) => (
                <li key={s.id}>
                  <button
                    type="button"
                    className={`kapi-pgx__item${activeId === `loose:${s.id}` ? " kapi-pgx__item--active" : ""}`}
                    onClick={() => loadLoose(s)}
                    aria-pressed={activeId === `loose:${s.id}`}
                  >
                    <span className="kapi-pgx__item-label">{s.label}</span>
                    <span className="kapi-pgx__item-kind">{s.kind}</span>
                    <code className="kapi-pgx__item-cmd">{s.suggested}</code>
                  </button>
                </li>
              ))}
            </ul>
          </section>

          <section className="kapi-pgx__group kapi-pgx__group--project">
            <h2 className="kapi-pgx__group-title">
              <FolderGit2 size={15} aria-hidden="true" />
              Sample projects
            </h2>
            <p className="kapi-pgx__group-sub">
              A recipe + content + a seeded content memory. Run the offline funnel.
            </p>
            <ul className="kapi-pgx__list">
              {PROJECT_SAMPLES.map((s) => (
                <li key={s.id}>
                  <button
                    type="button"
                    className={`kapi-pgx__item${activeId === `project:${s.id}` ? " kapi-pgx__item--active" : ""}`}
                    onClick={() => loadProject(s)}
                    aria-pressed={activeId === `project:${s.id}`}
                  >
                    <span className="kapi-pgx__item-label">{s.label}</span>
                    <span className="kapi-pgx__item-kind">{s.description}</span>
                    <code className="kapi-pgx__item-cmd">
                      add → extract → run translate → merge
                    </code>
                  </button>
                </li>
              ))}
            </ul>
            <p className="kapi-pgx__hint">
              <Play size={12} aria-hidden="true" /> After loading a project, run{" "}
              <code>kapi add</code>, <code>kapi memory import project.memory.json</code>,{" "}
              <code>kapi extract</code>, <code>kapi run translate</code>, then{" "}
              <code>kapi merge</code>.
            </p>
          </section>
        </aside>

        <div className="kapi-pgx__terminal">
          <KapiEmbed
            ref={embedRef}
            wasmExecUrl={cfg.wasmExecUrl}
            wasmUrl={cfg.wasmUrl}
            // Boot seeded with the first loose sample at the prompt.
            files={[LOOSE_SAMPLES[0].file]}
            cmd={LOOSE_SAMPLES[0].suggested}
            autoRun={false}
            bootOnMount
            showToolbar={false}
            fill
          />
        </div>
      </div>
    );
  }

  return { default: Explorer };
});

export default function KapiPlaygroundExplorer(): React.ReactElement {
  return (
    <PlaygroundDialog>
      <BrowserOnly fallback={<p role="status">Loading the in-browser terminal…</p>}>
        {() => (
          <ChunkSafeSuspense
            fallback={
              <p role="status" aria-live="polite">
                Loading the in-browser terminal…
              </p>
            }
          >
            <LazyExplorer />
          </ChunkSafeSuspense>
        )}
      </BrowserOnly>
    </PlaygroundDialog>
  );
}
