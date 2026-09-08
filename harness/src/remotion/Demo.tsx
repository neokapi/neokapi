import React from "react";
import { AbsoluteFill, interpolate, staticFile, useVideoConfig } from "remotion";
import { Audio } from "@remotion/media";
import type { BeatsFile, CapturedArtifact, CaptionsFile, DemoCapture, NarrationManifest, Screencast } from "../types.ts";
import { computeTiming, type SceneTiming } from "./timeline.ts";
import { cardsOf, highlightText, mergeScenes, presentationBetween, transitionFrames, type SceneSpec } from "./scene-plan.ts";
import { theme, setTheme, type ThemeMode } from "./components/theme.ts";
import { AUDIO_FADE_FRAMES, sceneLayout, type SceneLayout } from "./components/layout.ts";
import { ClaudeTerminal } from "./components/ClaudeTerminal.tsx";
import { PlainTerminal } from "./components/PlainTerminal.tsx";
import { TerminalWindow } from "./components/TerminalWindow.tsx";
import { TitleCard, OutroCard } from "./components/Cards.tsx";
import { ArtifactView } from "./components/ArtifactView.tsx";
import { PromptCard } from "./components/PromptCard.tsx";
import { DesktopScene } from "./components/DesktopScene.tsx";
import { Captions } from "./components/Captions.tsx";
import { ChapterLine } from "./components/Chapter.tsx";
import { SceneSeries, type SceneSlot } from "./SceneSeries.tsx";

export type DemoProps = {
  id: string;
  /** Loaded by Root's calculateMetadata from public/<id>/. */
  capture?: DemoCapture | null;
  narration?: NarrationManifest | null;
  beats?: BeatsFile | null;
  captions?: CaptionsFile | null;
  artifacts?: CapturedArtifact[];
  /** For terminal:"desktop" demos: the recorded screencast (beats + webms). */
  screencast?: Screencast | null;
  /** Which palette to render with (matches the docs page's light/dark mode). */
  themeMode?: ThemeMode;
  /** Narration locale (default "en"). Read by Root's calculateMetadata to load
   *  narration-<locale>.json; the narration prop already carries its audio paths. */
  locale?: string;
  /** Provenance stamp burned into a corner of every frame (version · sha · UTC). */
  stamp?: string;
};

/** Which scene kinds carry a chapter line above a window. */
const CHAPTERED: ReadonlySet<SceneSpec["kind"]> = new Set(["terminal", "artifact", "desktop"]);

export const Demo: React.FC<DemoProps> = ({ id, capture, narration, beats, captions, artifacts, screencast, themeMode, stamp }) => {
  // Swap the active palette before any child reads `theme.*`. The mode is constant
  // for the whole render job, so this is stable across frames.
  const mode: ThemeMode = themeMode ?? "dark";
  setTheme(mode);
  const { fps } = useVideoConfig();
  if (!capture || !narration) {
    return <AbsoluteFill style={{ background: theme.bg }} />;
  }
  const scenes = mergeScenes(narration, beats ?? null);
  const timing = computeTiming(scenes, fps, { transition: transitionFrames, events: capture.events });
  const cards = cardsOf(beats ?? null, capture);
  const shell = capture.terminal === "shell";
  const brand = capture.brand ?? (shell ? "kapi" : "claude");
  const beatById = new Map((screencast?.beats[mode] ?? []).map((b) => [b.id, b] as const));

  // The narration track fades in with the first spoken scene and out with the last.
  const spoken = scenes.map((s, i) => (s.text && (s.audio || s.audioFrom !== undefined) ? i : -1)).filter((i) => i >= 0);
  const firstSpoken = spoken[0] ?? -1;
  const lastSpoken = spoken[spoken.length - 1] ?? -1;
  const sceneAudio = (scene: SceneSpec, idx: number): React.ReactNode => {
    const fadeIn = idx === firstSpoken;
    const fadeOut = idx === lastSpoken;
    const volumeOver = (segFrames: number) => (f: number) => {
      let v = 1;
      if (fadeIn) v *= interpolate(f, [0, AUDIO_FADE_FRAMES], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
      if (fadeOut) v *= interpolate(f, [segFrames - AUDIO_FADE_FRAMES, segFrames], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" });
      return v;
    };
    if (narration.fullAudio && scene.audioFrom !== undefined && scene.audioTo !== undefined) {
      const from = Math.round(scene.audioFrom * fps);
      const to = Math.round(scene.audioTo * fps);
      if (to <= from) return null;
      return <Audio src={staticFile(`${id}/${narration.fullAudio}`)} trimBefore={from} trimAfter={to} volume={volumeOver(to - from)} name={`narration:${scene.id}`} />;
    }
    if (!narration.fullAudio && scene.audio) {
      return <Audio src={staticFile(`${id}/${scene.audio}`)} volume={volumeOver(Math.round(scene.durationSec * fps))} name={`narration:${scene.id}`} />;
    }
    return null;
  };
  // A one-shot narration without per-scene spans (an older narration.json)
  // plays as one continuous track from the first frame.
  const legacyTrack =
    narration.fullAudio && !scenes.some((s) => s.audioFrom !== undefined) ? (
      <Audio
        src={staticFile(`${id}/${narration.fullAudio}`)}
        volume={(f) =>
          Math.min(
            interpolate(f, [0, AUDIO_FADE_FRAMES], [0, 1], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }),
            interpolate(f, [timing.totalFrames - AUDIO_FADE_FRAMES, timing.totalFrames], [1, 0], { extrapolateLeft: "clamp", extrapolateRight: "clamp" }),
          )
        }
        name="narration"
      />
    ) : null;

  // The terminal scene, framed in the macOS window. Claude session or plain shell.
  const terminalScene = (scene: SceneSpec, t: SceneTiming, lay: SceneLayout) => (
    <TerminalWindow model={capture.meta.model} shell={shell} cwd={capture.cwd} layout={lay}>
      {shell ? (
        <PlainTerminal events={capture.events} revealStart={t.revealStart} revealEnd={t.revealEnd} highlight={highlightText(scene.highlight)} />
      ) : (
        <ClaudeTerminal events={capture.events} model={capture.meta.model} revealStart={t.revealStart} revealEnd={t.revealEnd} highlight={highlightText(scene.highlight)} />
      )}
    </TerminalWindow>
  );

  const renderScene = (scene: SceneSpec, idx: number, t: SceneTiming, lay: SceneLayout): React.ReactNode => {
    switch (scene.kind) {
      case "title":
        return <TitleCard title={cards.title} subtitle={cards.subtitle} brand={brand} />;
      case "prompt":
        return <PromptCard prompt={capture.prompt} />;
      case "outro":
        return <OutroCard line={cards.outroLine} pointer={cards.outroPointer} brand={brand} />;
      case "artifact": {
        const art = (artifacts ?? []).find((a) => a.id === scene.artifact);
        // Artifact failed to capture: fall back to the terminal so the scene isn't blank.
        if (!art) return terminalScene(scene, t, lay);
        return <ArtifactView demoId={id} artifact={art} layout={lay} highlight={scene.highlight?.box} />;
      }
      case "desktop": {
        const b = scene.beat ? beatById.get(scene.beat) : undefined;
        if (!screencast || !b) return terminalScene(scene, t, lay);
        const prev = scenes[idx - 1];
        const prevBeat = prev?.kind === "desktop" && prev.beat ? (beatById.get(prev.beat) ?? null) : null;
        return (
          <DesktopScene
            demoId={id}
            screencast={screencast}
            themeMode={mode}
            beat={b}
            prevBeat={prevBeat}
            sceneIndex={idx}
            globalFrom={t.from}
            sceneDurationFrames={t.durationFrames}
            layout={lay}
            crop={scene.crop}
            zoom={scene.zoom}
            highlight={scene.highlight?.box}
          />
        );
      }
      default:
        return terminalScene(scene, t, lay);
    }
  };

  const slots: SceneSlot[] = scenes.map((scene, idx) => {
    const t = timing.scenes[idx]!;
    const chapter = CHAPTERED.has(scene.kind) && scene.caption ? scene.caption : "";
    const lay = sceneLayout(chapter.length > 0);
    return {
      key: scene.id,
      name: `${scene.kind}:${scene.id}`,
      from: t.from,
      durationInFrames: t.durationFrames,
      transitionBefore: t.transitionBefore,
      transitionAfter: t.transitionAfter,
      presentationBefore: idx > 0 ? presentationBetween(scenes[idx - 1]!, scene) : undefined,
      presentationAfter: idx + 1 < scenes.length ? presentationBetween(scene, scenes[idx + 1]!) : undefined,
      children: (
        <>
          {sceneAudio(scene, idx)}
          {renderScene(scene, idx, t, lay)}
          {chapter ? <ChapterLine text={chapter} layout={lay} /> : null}
        </>
      ),
    };
  });

  return (
    <AbsoluteFill style={{ background: theme.bg, fontFamily: theme.fontSans }}>
      {legacyTrack}
      <SceneSeries slots={slots} />
      <Captions scenes={timing.scenes} captions={captions ?? null} />
      {stamp ? (
        <div
          style={{
            position: "absolute",
            right: 18,
            bottom: 14,
            zIndex: 100,
            fontFamily: theme.fontMono,
            fontSize: 13,
            lineHeight: 1,
            letterSpacing: 0.2,
            color: theme.text,
            background: theme.bg,
            opacity: 0.32,
            padding: "3px 7px",
            borderRadius: 5,
          }}
        >
          {stamp}
        </div>
      ) : null}
    </AbsoluteFill>
  );
};
