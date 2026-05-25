import React from "react";
import { AbsoluteFill, Audio, Sequence, staticFile } from "remotion";
import type { CapturedArtifact, DemoCapture, NarrationManifest } from "../types.ts";
import { computeTiming } from "./timeline.ts";
import { theme, setTheme, type ThemeMode } from "./components/theme.ts";
import { ClaudeTerminal } from "./components/ClaudeTerminal.tsx";
import { TerminalWindow } from "./components/TerminalWindow.tsx";
import { TitleCard, OutroCard } from "./components/Cards.tsx";
import { ArtifactView } from "./components/ArtifactView.tsx";
import { PromptCard } from "./components/PromptCard.tsx";

export interface DemoProps {
  id: string;
  capture: DemoCapture;
  narration: NarrationManifest;
  artifacts: CapturedArtifact[];
  /** Which palette to render with (matches the docs page's light/dark mode). */
  themeMode?: ThemeMode;
  // Remotion's Composition requires props to be assignable to Record<string, unknown>.
  [key: string]: unknown;
}

export const Demo: React.FC<DemoProps> = ({ id, capture, narration, artifacts, themeMode }) => {
  // Swap the active palette before any child reads `theme.*`. The mode is constant
  // for the whole render job, so this is stable across frames.
  setTheme(themeMode ?? "dark");
  const fps = 30;
  const timing = computeTiming(narration.scenes, fps);

  return (
    <AbsoluteFill style={{ background: theme.bg, fontFamily: theme.fontSans }}>
      {narration.scenes.map((scene, idx) => {
        const t = timing.scenes[idx];
        const audioSrc = scene.audio ? staticFile(`${id}/${scene.audio}`) : undefined;
        return (
          <Sequence key={scene.id} from={t.from} durationInFrames={t.durationFrames} name={`${scene.kind}:${scene.id}`}>
            {audioSrc ? <Audio src={audioSrc} /> : null}
            {scene.kind === "title" ? (
              <TitleCard title={capture.title} subtitle={capture.subtitle} tagline={capture.tagline} aspects={capture.aspects} />
            ) : scene.kind === "prompt" ? (
              <PromptCard prompt={capture.prompt} />
            ) : scene.kind === "outro" ? (
              <OutroCard title={capture.title} tagline={capture.tagline} aspects={capture.aspects} />
            ) : scene.kind === "artifact" ? (
              (() => {
                const art = artifacts.find((a) => a.id === scene.artifact);
                if (!art) {
                  // Artifact failed to capture — fall back to showing the terminal so the scene isn't blank.
                  return (
                    <TerminalWindow model={capture.meta.model} caption={scene.caption}>
                      <ClaudeTerminal events={capture.events} model={capture.meta.model} globalTermFrom={t.termFrom} totalTermFrames={timing.totalTermFrames} />
                    </TerminalWindow>
                  );
                }
                return <ArtifactView demoId={id} artifact={art} caption={scene.caption || art.caption} />;
              })()
            ) : (
              <TerminalWindow model={capture.meta.model} caption={scene.caption}>
                <ClaudeTerminal events={capture.events} model={capture.meta.model} globalTermFrom={t.termFrom} totalTermFrames={timing.totalTermFrames} />
              </TerminalWindow>
            )}
          </Sequence>
        );
      })}
    </AbsoluteFill>
  );
};
