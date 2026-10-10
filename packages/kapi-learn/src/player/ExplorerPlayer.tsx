import React, { useCallback, useMemo, useState } from "react";
import { RotateCcw, Share2 } from "lucide-react";
import { isBooted } from "@neokapi/kapi-playground/runtime";
import { PLUGIN_DESCRIPTORS } from "@neokapi/kapi-playground/plugins";
import type { Lab, SampleInfo, Series } from "../curriculum/types.ts";
import type { LabLink } from "../deeplink.ts";
import AboutLab from "./AboutLab.tsx";
import ChapterCard from "./ChapterCard.tsx";
import ChapterRail from "./ChapterRail.tsx";
import Poster from "./Poster.tsx";
import ShareMenu from "./ShareMenu.tsx";
import StageFrame from "./StageFrame.tsx";
import TryCard from "./TryCard.tsx";
import { useExplorerSession } from "./useExplorerSession.ts";
import type { LabAssets, UpNext } from "./types.ts";
import "./styles.css";

// The player for an explorer lab: the same chrome as the terminal labs, with
// one of the engine explorers on the stage. Each chapter resolves to an
// explorer and the props it shows; the host turns those into the component.
// A playground mounts the explorer the lab names, with the lab's own stage
// props, and offers what to try instead of chapters.

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
  const source = lab.chapters.length > 0 ? lab.chapters[index]?.stage : lab.stage;
  const { explorer, ...props } = source ?? {};
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
  const playground = lab.chapters.length === 0;
  const chapter = lab.chapters[session.current];
  const ready = session.status === "ready" || session.status === "running";
  const warm = typeof window !== "undefined" && isBooted();
  const total = lab.chapters.length;
  const atEnd = !playground && session.visited.size >= total;

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
      if (!ready || playground) return;
      if (e.key === "ArrowRight") {
        e.preventDefault();
        session.next();
      } else if (e.key === "ArrowLeft") {
        e.preventDefault();
        session.previous();
      }
    },
    [playground, ready, session],
  );

  return (
    <div
      className={`kl-player kl-player--${lab.sample} kl-player--explorer`}
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
            onClick={() => setStageEpoch((n) => n + 1)}
            disabled={!ready}
            title="Reload the explorer"
          >
            <RotateCcw size={15} aria-hidden="true" />
            Reload
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
              hasChanges={false}
            />
          </div>
        </div>
      </header>

      <div className={`kl-main${playground ? " kl-main--single" : ""}`}>
        <div className="kl-stage-col">
          <StageFrame expandable={ready} className="kl-stage--explorer" label="Explorer">
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
          </StageFrame>

          {chapter && (
            <ChapterCard
              chapter={chapter}
              index={session.current}
              total={total}
              ran={session.visited.has(chapter.id)}
              exitCode={null}
              ready={ready}
              busy={false}
              nextChapter={lab.chapters[session.current + 1]}
              atEnd={atEnd && session.current === total - 1}
              upNext={upNext}
              onPrevious={session.previous}
              onNext={session.next}
            />
          )}

          {(playground || (atEnd && session.current === total - 1)) &&
            lab.tryNext &&
            lab.tryNext.length > 0 && (
              <TryCard
                items={lab.tryNext}
                mode="prose"
                ready={ready}
                title={playground ? "Try" : "Try next"}
              />
            )}
        </div>

        {!playground && (
          <aside className="kl-rail" aria-label="Chapters">
            <p className="kl-rail__head">Chapters</p>
            <ChapterRail
              chapters={lab.chapters}
              current={session.current}
              executed={0}
              visited={session.visited}
              ready={ready}
              onSelect={session.goTo}
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
            : "Keys: the right arrow shows the next chapter, the left arrow goes back."
        }
      />
    </div>
  );
}
