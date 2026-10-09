import React, { useCallback, useMemo, useState } from "react";
import { BookOpen, Pause, Play, RotateCcw, Share2, SkipBack, SkipForward } from "lucide-react";
import { isBooted } from "@neokapi/kapi-playground/runtime";
import { PLUGIN_DESCRIPTORS } from "@neokapi/kapi-playground/plugins";
import type { Lab, SampleInfo, Series } from "../curriculum/types.ts";
import type { LabLink } from "../deeplink.ts";
import ChapterRail from "./ChapterRail.tsx";
import Narration from "./Narration.tsx";
import Poster from "./Poster.tsx";
import ShareMenu from "./ShareMenu.tsx";
import { useExplorerSession } from "./useExplorerSession.ts";
import type { LabAssets } from "./types.ts";
import type { UpNext } from "./LabPlayer.tsx";
import "./styles.css";

// The player for an explorer lab: the same chrome as the terminal labs, with
// one of the engine explorers on the stage. Each chapter resolves to an
// explorer and the props it shows; the host turns those into the component.

/** What a chapter puts on the stage, resolved: the explorer id and its logical props. */
export interface ResolvedStage {
  explorer: string;
  props: Record<string, unknown>;
}

export interface StageContext {
  assets: LabAssets;
  /** The engine and the lab's plugins are loaded. */
  ready: boolean;
}

export type StageRenderer = (stage: ResolvedStage, ctx: StageContext) => React.ReactNode;

export interface ExplorerPlayerProps {
  lab: Lab;
  series: Series;
  sample: SampleInfo;
  assets: LabAssets;
  link?: LabLink;
  onChapterChange?: (chapterId: string, index: number) => void;
  upNext?: UpNext;
  indexHref?: string;
  position?: { index: number; of: number };
  renderStage: StageRenderer;
}

function resolveStage(lab: Lab, index: number): ResolvedStage {
  const ch = lab.chapters[index];
  const { explorer, ...props } = ch?.stage ?? {};
  return { explorer: (explorer as string | undefined) ?? lab.explorer ?? "", props };
}

function mb(bytes?: number): string {
  if (!bytes) return "";
  return bytes >= 1_000_000_000
    ? `${(bytes / 1_000_000_000).toFixed(1)} GB`
    : `${Math.round(bytes / 1_000_000)} MB`;
}

export default function ExplorerPlayer({
  lab,
  series,
  sample,
  assets,
  link,
  onChapterChange,
  upNext,
  indexHref = "/learn",
  position,
  renderStage,
}: ExplorerPlayerProps): React.ReactElement {
  const session = useExplorerSession({
    lab,
    assets,
    link,
    onChapterChange: (ch, i) => onChapterChange?.(ch.id, i),
  });
  const [shareOpen, setShareOpen] = useState(false);
  const [stageEpoch, setStageEpoch] = useState(0);
  const chapter = lab.chapters[session.current];
  const ready = session.status === "ready" || session.status === "running";
  const warm = typeof window !== "undefined" && isBooted();
  const total = lab.chapters.length;

  const stage = useMemo(() => resolveStage(lab, session.current), [lab, session.current]);
  // The stage remounts only when what it shows changes, so a chapter that
  // only narrates leaves the explorer and its state alone.
  const stageKey = `${stageEpoch}:${stage.explorer}:${JSON.stringify(stage.props)}`;

  const hint = useMemo(() => {
    const parts: string[] = [];
    if (lab.engine !== false) parts.push("the kapi engine, a 20 MB download, once");
    for (const id of lab.plugins ?? []) {
      const d = PLUGIN_DESCRIPTORS.find((p) => p.id === id);
      if (d) parts.push(`the ${d.label} plugin${d.sizeBytes ? `, ${mb(d.sizeBytes)}` : ""}`);
    }
    if (parts.length === 0) {
      return "Runs in your browser. Models download the first time a step asks for them; nothing leaves your machine.";
    }
    return `Runs in your browser: loads ${parts.join(" and ")}. Nothing leaves your machine.`;
  }, [lab]);

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement;
      if (target.closest("input, textarea, select, [contenteditable], .kl-share, .xterm")) return;
      if (!ready) return;
      if (e.key === "ArrowRight") {
        e.preventDefault();
        session.next();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        session.previous();
      }
    },
    [ready, session],
  );

  return (
    <div
      className={`kl-player kl-player--${lab.sample} kl-player--explorer`}
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
            onClick={() => setStageEpoch((n) => n + 1)}
            disabled={!ready}
            title="Reload the explorer for this chapter"
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
              hasChanges={false}
            />
          </div>
        </div>
      </header>

      <div className="kl-main">
        <div className="kl-stage-col">
          <div className="kl-stage kl-stage--explorer" tabIndex={-1}>
            {ready && (
              <div className="kl-stage__body" key={stageKey}>
                {renderStage(stage, { assets, ready })}
              </div>
            )}
            {!ready && (
              <Poster
                lab={lab}
                sample={sample}
                seriesTitle={series.title}
                status={session.status}
                bootProgress={session.bootProgress}
                error={session.error}
                warm={warm && lab.engine !== false}
                onStart={session.start}
                hint={hint}
                loadingLabel={session.loading ? `Loading ${session.loading}` : undefined}
              />
            )}
          </div>

          <div className="kl-transport" role="group" aria-label="Lab transport">
            <div className="kl-transport__buttons">
              <button
                type="button"
                className="kl-tbtn"
                onClick={session.previous}
                disabled={!ready || session.current === 0}
                aria-label="Previous chapter"
                title="Previous chapter (←)"
              >
                <SkipBack size={18} aria-hidden="true" />
              </button>
              {session.playing ? (
                <button
                  type="button"
                  className="kl-tbtn kl-tbtn--main"
                  onClick={session.pause}
                  aria-label="Pause"
                >
                  <Pause size={22} aria-hidden="true" fill="currentColor" />
                </button>
              ) : (
                <button
                  type="button"
                  className="kl-tbtn kl-tbtn--main"
                  onClick={session.play}
                  disabled={!ready || session.current >= total - 1}
                  aria-label="Play: walk the chapters with a pause to read"
                >
                  <Play size={22} aria-hidden="true" fill="currentColor" />
                </button>
              )}
              <button
                type="button"
                className="kl-tbtn"
                onClick={session.next}
                disabled={!ready || session.current >= total - 1}
                aria-label="Next chapter"
                title="Next chapter (→)"
              >
                <SkipForward size={18} aria-hidden="true" />
              </button>
            </div>
            <div className="kl-scrub" role="list" aria-label="Chapters">
              {lab.chapters.map((ch, i) => {
                const seen = session.visited.has(ch.id);
                return (
                  <button
                    key={ch.id}
                    type="button"
                    role="listitem"
                    className={`kl-scrub__seg kl-scrub__seg--${seen ? "done" : "pending"}${i === session.current ? " kl-scrub__seg--current" : ""}`}
                    onClick={() => session.goTo(i)}
                    disabled={!ready}
                    aria-label={`Chapter ${i + 1}: ${ch.title}`}
                    aria-current={i === session.current ? "step" : undefined}
                    title={`${i + 1}. ${ch.title}`}
                  />
                );
              })}
            </div>
            <div className="kl-transport__counter" aria-live="polite">
              {session.current + 1} / {total}
            </div>
          </div>

          {chapter && (
            <Narration
              chapter={chapter}
              index={session.current}
              total={total}
              done={session.visited.has(chapter.id)}
              exitCode={null}
              onOpenFile={() => {}}
            />
          )}
        </div>

        <aside className="kl-rail" aria-label="Chapters and more">
          <div className="kl-rail__head">
            <span>Chapters</span>
            <span className="kl-rail__count">
              {session.visited.size}/{total} seen
            </span>
          </div>
          <ChapterRail
            chapters={lab.chapters}
            current={session.current}
            executed={0}
            visited={session.visited}
            ready={ready}
            onSelect={session.goTo}
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
                <h4 className="kl-about__sub">Try next</h4>
                <ul className="kl-try kl-try--prose">
                  {lab.tryNext.map((t) => (
                    <li key={t}>{t}</li>
                  ))}
                </ul>
              </>
            )}
            <p className="kl-about__keys">
              Keys: <kbd>→</kbd> next · <kbd>←</kbd> back
            </p>
          </section>
        </aside>
      </div>
    </div>
  );
}
