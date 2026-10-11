import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { FolderOpen, RotateCcw, Share2 } from "lucide-react";
import { isBooted } from "@neokapi/kapi-playground/runtime";
import { LENSES } from "../lens/index.ts";
import type { Focus } from "../lens/index.ts";
import AboutLab from "./AboutLab.tsx";
import FilesPane from "./FilesPane.tsx";
import LearnTerminal from "./LearnTerminal.tsx";
import Poster from "./Poster.tsx";
import ShareMenu from "./ShareMenu.tsx";
import StageFrame from "./StageFrame.tsx";
import StepCard from "./StepCard.tsx";
import Stepper from "./Stepper.tsx";
import TryCard from "./TryCard.tsx";
import { useLabSession } from "./useLabSession.ts";
import { useLens } from "./useLens.ts";
import type { LabPlayerProps } from "./LabPlayer.tsx";
import "./styles.css";

// The tour layout for a terminal lab: the steps as chips across the top, the
// step's card beside the stage with the one control that runs it, and the
// files pane under both. Play boots the engine and leaves the first step in
// view. Nothing runs until the reader presses Run, so each step is read
// before it runs and again after, with its output on the stage. The standard
// player (LabPlayer) drives the same session hook and runs a chapter as the
// reader moves to it.

export type LabPlayerTourProps = LabPlayerProps;

export default function LabPlayerTour({
  lab,
  series,
  sample,
  assets,
  link,
  onChapterChange,
  upNext,
  indexHref = "/learn",
  position,
}: LabPlayerTourProps): React.ReactElement {
  const session = useLabSession({
    lab,
    assets,
    link,
    runFirst: false,
    onChapterChange: (ch, i) => onChapterChange?.(ch.id, i),
  });
  const [shareOpen, setShareOpen] = useState(false);
  const [filesOpen, setFilesOpen] = useState(true);
  const playground = lab.chapters.length === 0;
  const total = lab.chapters.length;
  const chapter = lab.chapters[session.current];
  const ready = session.status === "ready" || session.status === "running";
  const warm = typeof window !== "undefined" && isBooted();
  const ran = session.current < session.executed;
  const atEnd = !playground && session.executed >= total;

  // The files pane opens itself when a step points at a file.
  useEffect(() => {
    if (session.selectedFile) setFilesOpen(true);
  }, [session.selectedFile]);

  // A step action replaces the control that had focus: Run becomes Next, and
  // a chip is disabled while the steps before it replay. Focus would drop to
  // the page and take the keys with it, so it goes to the card instead,
  // unless the reader has it somewhere, the terminal above all.
  const cardRef = useRef<HTMLElement>(null);
  useEffect(() => {
    if (!ready || session.busy) return;
    const active = document.activeElement;
    if (!active || active === document.body) cardRef.current?.focus({ preventScroll: true });
  }, [ready, session.busy, session.current, session.executed]);

  const lastExitFor = useMemo(() => {
    const entry = [...session.transcript].reverse().find((t) => t.chapterId === chapter?.id);
    return entry?.code ?? null;
  }, [session.transcript, chapter]);

  // The right arrow does the next thing: it runs the step in view when that
  // has not run, and moves on when it has.
  const forward = useCallback(() => {
    if (ran) void session.show(session.current + 1);
    else void session.run();
  }, [ran, session]);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement;
      if (target.closest(".xterm, input, textarea, [contenteditable], .kl-share")) return;
      if (!ready || playground) return;
      if (e.key === "ArrowRight") {
        e.preventDefault();
        forward();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        session.previous();
      } else if (
        (e.key === "Enter" || e.key === " ") &&
        !target.closest("button, a, summary, [role=button]")
      ) {
        // Enter on a control is the control's own; elsewhere it runs the step.
        e.preventDefault();
        void session.run();
      }
    },
    [forward, playground, ready, session],
  );

  const hasChanges = session.files.some((f) => f.changed);
  const tryItems = lab.tryNext ?? [];
  const lens = lab.lens ? LENSES[lab.lens] : undefined;
  const { graph, previous, loading: graphBusy } = useLens(lens, session, ready);

  const poster = (
    <Poster
      lab={lab}
      sample={sample}
      seriesTitle={series.title}
      status={session.status}
      bootProgress={session.bootProgress}
      error={session.error}
      warm={warm}
      onStart={session.start}
    />
  );

  return (
    <div
      className={`kl-player klt kl-player--${lab.sample}${lens ? " kl-player--lens" : ""}`}
      data-status={session.status}
      onKeyDown={onKeyDown}
    >
      <header className="kl-head">
        <div className="kl-head__text">
          <p className="kl-head__crumbs">
            <a href={indexHref}>Learn kapi</a>
            <span aria-hidden="true">/</span>
            <span>{series.title}</span>
            {position && (
              <span className="kl-head__pos">
                Lab {position.index} of {position.of}
              </span>
            )}
          </p>
          <h1 className="kl-head__title">{lab.title}</h1>
          <p className="kl-head__tagline">{lab.tagline}</p>
        </div>
        <div className="kl-head__actions">
          <button
            type="button"
            className="kl-btn"
            onClick={() => setFilesOpen((v) => !v)}
            aria-pressed={filesOpen}
            title="Show or hide the files pane"
          >
            <FolderOpen size={15} aria-hidden="true" />
            Files
          </button>
          <button
            type="button"
            className="kl-btn"
            onClick={() => void session.reset()}
            disabled={!ready || session.busy}
            title="Start the lab over with a fresh sandbox"
          >
            <RotateCcw size={15} aria-hidden="true" />
            Start over
          </button>
          <div className="kl-share-anchor">
            <button
              type="button"
              className="kl-btn"
              onClick={() => setShareOpen((v) => !v)}
              aria-expanded={shareOpen}
              aria-haspopup="dialog"
            >
              <Share2 size={15} aria-hidden="true" />
              Share
            </button>
            <ShareMenu
              open={shareOpen}
              onClose={() => setShareOpen(false)}
              chapterTitle={chapter ? `step ${session.current + 1}, ${chapter.title}` : lab.title}
              makeLink={session.shareLink}
              hasChanges={hasChanges}
            />
          </div>
        </div>
      </header>

      {!playground && (
        <Stepper
          chapters={lab.chapters}
          current={session.current}
          executed={session.executed}
          ready={ready}
          busy={session.busy}
          onSelect={(i) => void session.show(i)}
        />
      )}

      <div className="klt-body">
        <div className="klt-main">
          <div className="klt-side">
            {chapter ? (
              <StepCard
                ref={cardRef}
                chapter={chapter}
                index={session.current}
                total={total}
                ran={ran}
                exitCode={lastExitFor}
                ready={ready}
                busy={session.busy}
                nextChapter={lab.chapters[session.current + 1]}
                atEnd={atEnd}
                upNext={upNext}
                onPrevious={session.previous}
                onRun={() => void session.run()}
                onNext={() => void session.show(session.current + 1)}
                onReplay={() => void session.reset()}
                onOpenFile={(p) => {
                  session.selectFile(p, chapter.look?.view);
                  setFilesOpen(true);
                }}
              />
            ) : (
              <TryCard
                items={tryItems}
                mode="commands"
                ready={ready}
                busy={session.busy}
                onRun={(cmd) => void session.typeLine(cmd)}
                title="Try"
                lead="Press a command to run it, or type your own at the prompt. The files pane takes your own files too."
              />
            )}
          </div>

          <div className="klt-stage-col">
            {lens && (
              <StageFrame expandable={ready} className="kl-stage--lens" label={lens.label}>
                {ready ? (
                  <lens.Component
                    model={graph}
                    previous={previous}
                    focus={(chapter?.focus as Focus | undefined) ?? "all"}
                    loading={graphBusy}
                  />
                ) : (
                  poster
                )}
              </StageFrame>
            )}
            <StageFrame expandable={ready} className="kl-stage--terminal" label="Terminal">
              {session.status !== "idle" && (
                <LearnTerminal
                  className="kl-terminal"
                  onSubmit={session.runLine}
                  onReady={session.registerTerminal}
                  promptLabel={session.promptLabel}
                />
              )}
              {!ready && !lens && poster}
            </StageFrame>
          </div>
        </div>

        {atEnd && (
          <TryCard
            items={tryItems}
            mode="commands"
            ready={ready}
            busy={session.busy}
            onRun={(cmd) => void session.typeLine(cmd)}
            title="Try next"
          />
        )}

        {filesOpen && (
          <FilesPane
            files={session.files}
            lastChanged={session.lastChanged}
            selected={session.selectedFile}
            onSelect={(p) => session.selectFile(p)}
            view={session.selectedView}
            showHidden={session.showHidden}
            onShowHidden={session.setShowHidden}
            readFile={session.readFile}
            inspect={session.inspect}
            onAddFiles={ready ? session.addFiles : undefined}
            version={session.version}
          />
        )}
      </div>

      <AboutLab
        lab={lab}
        sample={sample}
        upNext={upNext}
        keys={
          playground
            ? undefined
            : "Keys: enter runs the step in view, the right arrow runs it or moves on, the left arrow goes back."
        }
      />
    </div>
  );
}
