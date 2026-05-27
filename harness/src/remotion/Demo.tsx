import React from "react";
import { AbsoluteFill, Audio, Sequence, staticFile } from "remotion";
import type { CapturedArtifact, DemoCapture, NarrationManifest, Screencast } from "../types.ts";
import { computeTiming } from "./timeline.ts";
import { theme, setTheme, type ThemeMode } from "./components/theme.ts";
import { ClaudeTerminal } from "./components/ClaudeTerminal.tsx";
import { PlainTerminal } from "./components/PlainTerminal.tsx";
import { TerminalWindow } from "./components/TerminalWindow.tsx";
import { TitleCard, OutroCard } from "./components/Cards.tsx";
import { ArtifactView } from "./components/ArtifactView.tsx";
import { PromptCard } from "./components/PromptCard.tsx";
import { DesktopScene } from "./components/DesktopScene.tsx";

export interface DemoProps {
  id: string;
  capture: DemoCapture;
  narration: NarrationManifest;
  artifacts: CapturedArtifact[];
  /** For terminal:"desktop" demos: the recorded screencast (beats + webms). */
  screencast?: Screencast | null;
  /** Which palette to render with (matches the docs page's light/dark mode). */
  themeMode?: ThemeMode;
  // Remotion's Composition requires props to be assignable to Record<string, unknown>.
  [key: string]: unknown;
}

export const Demo: React.FC<DemoProps> = ({ id, capture, narration, artifacts, screencast, themeMode }) => {
  // Swap the active palette before any child reads `theme.*`. The mode is constant
  // for the whole render job, so this is stable across frames.
  setTheme(themeMode ?? "dark");
  const fps = 30;
  const timing = computeTiming(narration.scenes, fps);
  const shell = capture.terminal === "shell";
  const brand = capture.brand ?? (shell ? "kapi" : "claude");

  // The terminal scene, framed in the macOS window. Claude session or plain shell.
  const terminalScene = (caption: string, termFrom: number) => (
    <TerminalWindow model={capture.meta.model} caption={caption} shell={shell} cwd={capture.cwd}>
      {shell ? (
        <PlainTerminal events={capture.events} globalTermFrom={termFrom} totalTermFrames={timing.totalTermFrames} />
      ) : (
        <ClaudeTerminal events={capture.events} model={capture.meta.model} globalTermFrom={termFrom} totalTermFrames={timing.totalTermFrames} />
      )}
    </TerminalWindow>
  );

  // One-shot narration: a single continuous track for the whole video (uniform
  // tempo/tone). Otherwise each scene carries its own clip.
  const fullAudio = narration.fullAudio;

  // Desktop screencast beats (per active theme). Each desktop scene eases in to
  // its zoom region and back out to full on its own (see DesktopScene).
  const mode: ThemeMode = themeMode ?? "dark";
  const beats = screencast?.beats[mode] ?? [];
  const beatById = new Map(beats.map((b) => [b.id, b] as const));

  return (
    <AbsoluteFill style={{ background: theme.bg, fontFamily: theme.fontSans }}>
      {fullAudio ? <Audio src={staticFile(`${id}/${fullAudio}`)} /> : null}
      {narration.scenes.map((scene, idx) => {
        const t = timing.scenes[idx];
        const audioSrc = !fullAudio && scene.audio ? staticFile(`${id}/${scene.audio}`) : undefined;
        return (
          <Sequence key={scene.id} from={t.from} durationInFrames={t.durationFrames} name={`${scene.kind}:${scene.id}`}>
            {audioSrc ? <Audio src={audioSrc} /> : null}
            {scene.kind === "title" ? (
              <TitleCard title={capture.title} subtitle={capture.subtitle} tagline={capture.tagline} aspects={capture.aspects} brand={brand} />
            ) : scene.kind === "prompt" ? (
              <PromptCard prompt={capture.prompt} />
            ) : scene.kind === "outro" ? (
              <OutroCard title={capture.title} tagline={capture.tagline} aspects={capture.aspects} brand={brand} />
            ) : scene.kind === "artifact" ? (
              (() => {
                const art = artifacts.find((a) => a.id === scene.artifact);
                // Artifact failed to capture — fall back to the terminal so the scene isn't blank.
                if (!art) return terminalScene(scene.caption, t.termFrom);
                return <ArtifactView demoId={id} artifact={art} caption={scene.caption || art.caption} />;
              })()
            ) : scene.kind === "desktop" ? (
              (() => {
                const b = scene.beat ? beatById.get(scene.beat) : undefined;
                if (!screencast || !b) return terminalScene(scene.caption, t.termFrom);
                return (
                  <DesktopScene
                    demoId={id}
                    screencast={screencast}
                    themeMode={mode}
                    beat={b}
                    caption={scene.caption}
                    sceneDurationFrames={t.durationFrames}
                  />
                );
              })()
            ) : (
              terminalScene(scene.caption, t.termFrom)
            )}
          </Sequence>
        );
      })}
    </AbsoluteFill>
  );
};
