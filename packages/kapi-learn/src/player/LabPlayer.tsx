import React, { useCallback, useEffect, useMemo, useState } from "react";
import { BookOpen, FolderOpen, RotateCcw, Share2 } from "lucide-react";
import { isBooted } from "@neokapi/kapi-playground/runtime";
import type { Lab, SampleInfo, Series } from "../curriculum/types.ts";
import type { LabLink } from "../deeplink.ts";
import ChapterRail from "./ChapterRail.tsx";
import FilesPane from "./FilesPane.tsx";
import LearnTerminal from "./LearnTerminal.tsx";
import Narration from "./Narration.tsx";
import Poster from "./Poster.tsx";
import ShareMenu from "./ShareMenu.tsx";
import Transport from "./Transport.tsx";
import { useLabSession } from "./useLabSession.ts";
import type { LabAssets } from "./types.ts";
import "./styles.css";

// The lab player. A video site's shape: the stage (the terminal) with a
// transport bar and the chapter's narration under it, the chapter list and
// "up next" beside it, and the files pane below. Nothing is fetched until
// Play; a deep link opens at its chapter once the engine is up.

export interface UpNext {
  id: string;
  title: string;
  tagline: string;
  href: string;
}

export interface LabPlayerProps {
  lab: Lab;
  series: Series;
  sample: SampleInfo;
  assets: LabAssets;
  /** The deep link the page opened with (chapter, shared files). */
  link?: LabLink;
  /** Called when the chapter in view changes, so the host keeps the URL current. */
  onChapterChange?: (chapterId: string, index: number) => void;
  /** The next lab in the curriculum, for the "up next" card. */
  upNext?: UpNext;
  /** Where the lab's index lives, for the back link. */
  indexHref?: string;
  /** Position of this lab in its series and the series length, for the eyebrow. */
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
  const chapter = lab.chapters[session.current];
  const ready = session.status === "ready" || session.status === "running";
  const warm = typeof window !== "undefined" && isBooted();

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
      if (!ready) return;
      if (e.key === " " || e.key === "k") {
        e.preventDefault();
        if (session.playing) session.pause();
        else session.play();
      } else if (e.key === "ArrowRight" || e.key === "l") {
        e.preventDefault();
        void session.stepForward();
      } else if (e.key === "ArrowLeft" || e.key === "j") {
        e.preventDefault();
        session.stepBack();
      } else if (e.key === "Enter") {
        e.preventDefault();
        void session.runCurrent();
      }
    },
    [ready, session],
  );

  const hasChanges = session.files.some((f) => f.changed);

  return (
    <div
      className={`kl-player kl-player--${lab.sample}`}
      data-status={session.status}
      onKeyDown={onKeyDown}
    >
      <header className="kl-head">
        <div className="kl-head__text">
          <p className="kl-head__eyebrow">
            <a href={indexHref}>Learn kapi</a>
            <span aria-hidden="true">›</span>
            <span>{series.title}</span>
            {position && (
              <>
                <span aria-hidden="true">·</span>
                <span>
                  Lab {position.index} of {position.of}
                </span>
              </>
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
            Reset
          </button>
          <div className="kl-share-anchor">
            <button
              type="button"
              className="kl-btn kl-btn--primary"
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

      <div className="kl-main">
        <div className="kl-stage-col">
          <div className="kl-stage" tabIndex={-1}>
            {session.status !== "idle" && (
              <LearnTerminal
                className="kl-terminal"
                onSubmit={session.runLine}
                onReady={session.registerTerminal}
                promptLabel={session.promptLabel}
                onUserCommand={session.pause}
              />
            )}
            {!ready && (
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
          </div>

          <Transport
            chapters={lab.chapters}
            current={session.current}
            executed={session.executed}
            playing={session.playing}
            busy={session.busy}
            ready={ready}
            onPlay={session.play}
            onPause={session.pause}
            onRunCurrent={() => void session.runCurrent()}
            onBack={session.stepBack}
            onForward={() => void session.stepForward()}
            onSeek={(i) => void session.goTo(i)}
          />

          {ready && session.executed >= lab.chapters.length && (
            <div className="kl-complete" role="status">
              <span className="kl-complete__text">
                Every chapter has run. The terminal is yours: try the commands under “Try next”, or
                go on.
              </span>
              <span className="kl-complete__actions">
                <button
                  type="button"
                  className="kl-btn"
                  onClick={() => void session.reset().then(() => session.play())}
                  disabled={session.busy}
                >
                  Play again
                </button>
                {upNext && (
                  <a className="kl-btn kl-btn--primary" href={upNext.href}>
                    Next lab: {upNext.title}
                  </a>
                )}
              </span>
            </div>
          )}

          {chapter && (
            <Narration
              chapter={chapter}
              index={session.current}
              total={lab.chapters.length}
              done={session.current < session.executed}
              exitCode={lastExitFor}
              onOpenFile={(p) => {
                session.selectFile(p, chapter.look?.view);
                setFilesOpen(true);
              }}
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

        <aside className="kl-rail" aria-label="Chapters and more">
          <div className="kl-rail__head">
            <span>Chapters</span>
            <span className="kl-rail__count">
              {session.executed}/{lab.chapters.length} run
            </span>
          </div>
          <ChapterRail
            chapters={lab.chapters}
            current={session.current}
            executed={session.executed}
            ready={ready}
            onSelect={(i) => void session.goTo(i)}
          />

          {upNext && (
            <a className="kl-upnext" href={upNext.href}>
              <span className="kl-upnext__eyebrow">Up next</span>
              <span className="kl-upnext__title">{upNext.title}</span>
              <span className="kl-upnext__tagline">{upNext.tagline}</span>
            </a>
          )}

          <section className="kl-about">
            <h3 className="kl-about__title">
              <BookOpen size={14} aria-hidden="true" /> About this lab
            </h3>
            <p>{lab.summary}</p>
            <p className="kl-about__sample">
              <strong>{sample.name}.</strong> {sample.blurb}
            </p>
            <ul className="kl-concepts" aria-label="Concepts">
              {lab.concepts.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
            <ul className="kl-docs" aria-label="Read on">
              {lab.docs.map((d) => (
                <li key={d.href}>
                  <a href={d.href}>{d.label}</a>
                </li>
              ))}
            </ul>
            {lab.tryNext && lab.tryNext.length > 0 && (
              <>
                <h4 className="kl-about__sub">Try next, in the terminal</h4>
                <ul className="kl-try">
                  {lab.tryNext.map((t) => (
                    <li key={t}>
                      <code>{t}</code>
                    </li>
                  ))}
                </ul>
              </>
            )}
            <p className="kl-about__keys">
              Keys: <kbd>space</kbd> play or pause · <kbd>→</kbd> next · <kbd>←</kbd> back ·{" "}
              <kbd>enter</kbd> run this chapter
            </p>
          </section>
        </aside>
      </div>
    </div>
  );
}
