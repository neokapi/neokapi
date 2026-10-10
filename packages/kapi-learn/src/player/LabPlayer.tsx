import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { FolderOpen, RotateCcw, Share2 } from "lucide-react";
import { isBooted } from "@neokapi/kapi-playground/runtime";
import type { Lab, SampleInfo, Series } from "../curriculum/types.ts";
import type { LabLink } from "../deeplink.ts";
import { LENSES } from "../lens/index.ts";
import type { Focus, GraphModel } from "../lens/index.ts";
import AboutLab from "./AboutLab.tsx";
import ChapterCard from "./ChapterCard.tsx";
import ChapterRail from "./ChapterRail.tsx";
import FilesPane from "./FilesPane.tsx";
import LearnTerminal from "./LearnTerminal.tsx";
import Poster from "./Poster.tsx";
import ShareMenu from "./ShareMenu.tsx";
import StageFrame from "./StageFrame.tsx";
import TryCard from "./TryCard.tsx";
import { useLabSession } from "./useLabSession.ts";
import type { LabAssets, UpNext } from "./types.ts";
import "./styles.css";

// The lab player for a terminal lab. The stage is the terminal; under it,
// the chapter card says what the chapter did and carries Previous and Next;
// beside it, the chapter list. A playground skips the card for a list of
// things to try. A lab with a lens draws it above the terminal and rebuilds
// it after every command. Nothing is fetched until Play; a deep link opens at
// its chapter once the engine is up.

export type { UpNext };

export interface LabPlayerProps {
  lab: Lab;
  series: Series;
  sample: SampleInfo;
  assets: LabAssets;
  /** The deep link the page opened with (chapter, shared files). */
  link?: LabLink;
  /** Called when the chapter in view changes, so the host keeps the URL current. */
  onChapterChange?: (chapterId: string, index: number) => void;
  /** The next lab in the curriculum. */
  upNext?: UpNext;
  /** Where the lab's index lives, for the back link. */
  indexHref?: string;
  /** Position of this lab in its series and the series length. */
  position?: { index: number; of: number };
}

export default function LabPlayer({
  lab,
  series,
  sample,
  assets,
  link,
  onChapterChange,
  upNext,
  indexHref = "/learn",
  position,
}: LabPlayerProps): React.ReactElement {
  const session = useLabSession({
    lab,
    assets,
    link,
    onChapterChange: (ch, i) => onChapterChange?.(ch.id, i),
  });
  const [shareOpen, setShareOpen] = useState(false);
  const [filesOpen, setFilesOpen] = useState(true);
  const playground = lab.chapters.length === 0;
  const chapter = lab.chapters[session.current];
  const ready = session.status === "ready" || session.status === "running";
  const warm = typeof window !== "undefined" && isBooted();
  const atEnd = !playground && session.executed >= lab.chapters.length;

  // The files pane opens itself when a chapter points at a file.
  useEffect(() => {
    if (session.selectedFile) setFilesOpen(true);
  }, [session.selectedFile]);

  const lastExitFor = useMemo(() => {
    const entry = [...session.transcript].reverse().find((t) => t.chapterId === chapter?.id);
    return entry?.code ?? null;
  }, [session.transcript, chapter]);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement;
      if (target.closest(".xterm, input, textarea, [contenteditable], .kl-share")) return;
      if (!ready || playground) return;
      if (e.key === "ArrowRight") {
        e.preventDefault();
        void session.next();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        session.previous();
      }
    },
    [playground, ready, session],
  );

  const hasChanges = session.files.some((f) => f.changed);
  const tryItems = lab.tryNext ?? [];

  // The lens: rebuilt from the engine's answers after every command settles.
  const lens = lab.lens ? LENSES[lab.lens] : undefined;
  const [graph, setGraph] = useState<GraphModel | null>(null);
  const [graphBusy, setGraphBusy] = useState(false);
  const graphRef = useRef<GraphModel | null>(null);
  const previousRef = useRef<GraphModel | null>(null);
  const { runCapture, version, busy } = session;
  useEffect(() => {
    if (!lens || !ready || busy) return;
    let cancelled = false;
    const h = setTimeout(() => {
      setGraphBusy(true);
      void lens
        .build(runCapture)
        .then((m) => {
          if (cancelled) return;
          previousRef.current = graphRef.current;
          graphRef.current = m;
          setGraph(m);
        })
        .finally(() => !cancelled && setGraphBusy(false));
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(h);
    };
  }, [lens, ready, busy, version, runCapture]);

  // The chapter card sits under the terminal, where the output it explains
  // is; with a lens the view above the terminal is what the chapter changes,
  // so the card moves between the two and the reader presses Next with the
  // graph in sight.
  const chapterCard = chapter ? (
    <ChapterCard
      chapter={chapter}
      index={session.current}
      total={lab.chapters.length}
      ran={session.current < session.executed}
      exitCode={lastExitFor}
      ready={ready}
      busy={session.busy}
      nextChapter={lab.chapters[session.current + 1]}
      atEnd={atEnd}
      upNext={upNext}
      onPrevious={session.previous}
      onNext={() => void session.next()}
      onReplay={() => void session.reset()}
      onOpenFile={(p) => {
        session.selectFile(p, chapter.look?.view);
        setFilesOpen(true);
      }}
    />
  ) : null;

  return (
    <div
      className={`kl-player kl-player--${lab.sample}${lens ? " kl-player--lens" : ""}`}
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
              chapterTitle={
                chapter ? `chapter ${session.current + 1}, ${chapter.title}` : lab.title
              }
              makeLink={session.shareLink}
              hasChanges={hasChanges}
            />
          </div>
        </div>
      </header>

      {lens && (
        <StageFrame expandable={ready} className="kl-stage--lens" label={lens.label}>
          {ready ? (
            <lens.Component
              model={graph}
              previous={previousRef.current}
              focus={(chapter?.focus as Focus | undefined) ?? "all"}
              loading={graphBusy}
            />
          ) : (
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
          )}
        </StageFrame>
      )}

      <div className={`kl-main${playground ? " kl-main--single" : ""}`}>
        <div className="kl-stage-col">
          {lens && chapterCard}
          <StageFrame expandable={ready} className="kl-stage--terminal" label="Terminal">
            {session.status !== "idle" && (
              <LearnTerminal
                className="kl-terminal"
                onSubmit={session.runLine}
                onReady={session.registerTerminal}
                promptLabel={session.promptLabel}
              />
            )}
            {!ready && !lens && (
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
            )}
          </StageFrame>

          {!lens && chapterCard}

          {(playground || atEnd) && (
            <TryCard
              items={tryItems}
              mode="commands"
              ready={ready}
              busy={session.busy}
              onRun={(cmd) => void session.typeLine(cmd)}
              title={playground ? "Try" : "Try next"}
              lead={
                playground
                  ? "Press a command to run it, or type your own at the prompt. The files pane takes your own files too."
                  : undefined
              }
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

        {!playground && (
          <aside className="kl-rail" aria-label="Chapters">
            <p className="kl-rail__head">Chapters</p>
            <ChapterRail
              chapters={lab.chapters}
              current={session.current}
              executed={session.executed}
              ready={ready}
              onSelect={(i) => void session.goTo(i)}
            />
          </aside>
        )}
      </div>

      <AboutLab
        lab={lab}
        sample={sample}
        upNext={upNext}
        keys={
          playground
            ? undefined
            : "Keys: the right arrow runs the next chapter, the left arrow goes back."
        }
      />
    </div>
  );
}
