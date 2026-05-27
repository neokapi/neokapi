import React from "react";
import {
  AbsoluteFill,
  OffthreadVideo,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";
import type { Screencast, ScreencastBeat, ZoomRect } from "../../types.ts";
import { theme } from "./theme.ts";
import type { ThemeMode } from "./theme.ts";

/** A macOS traffic-light dot. */
const Light: React.FC<{ c: string }> = ({ c }) => (
  <span style={{ width: 13, height: 13, borderRadius: 13, background: c, display: "inline-block", boxShadow: "0 0 0 0.5px rgba(0,0,0,0.18)" }} />
);

interface Transform {
  scale: number;
  tx: number;
  ty: number;
}

/** Transform that magnifies a normalized zoom rect to fill the body, centered + edge-clamped. */
function zoomTransform(zoom: ZoomRect | null, bw: number, bh: number): Transform {
  if (!zoom) return { scale: 1, tx: 0, ty: 0 };
  const rw = zoom.w * bw;
  const rh = zoom.h * bh;
  const scale = Math.min(2.6, Math.max(1, Math.min(bw / rw, bh / rh)));
  const rcx = (zoom.x + zoom.w / 2) * bw;
  const rcy = (zoom.y + zoom.h / 2) * bh;
  let tx = bw / 2 - rcx * scale;
  let ty = bh / 2 - rcy * scale;
  tx = Math.min(0, Math.max(bw - bw * scale, tx));
  ty = Math.min(0, Math.max(bh - bh * scale, ty));
  return { scale, tx, ty };
}

const lerp = (a: number, b: number, t: number) => a + (b - a) * t;

/**
 * One Kapi Desktop walkthrough beat: the recorded screencast slice for `beat`,
 * played inside a unified macOS window (the app's own top bar IS the title bar;
 * the traffic lights sit inline over its top-left). The camera eases IN to the
 * beat's zoom region, holds, then eases back OUT to the full frame before the
 * next beat — so the view keeps returning to the whole app.
 */
export const DesktopScene: React.FC<{
  demoId: string;
  screencast: Screencast;
  themeMode: ThemeMode;
  beat: ScreencastBeat;
  caption: string;
  sceneDurationFrames: number;
}> = ({ demoId, screencast, themeMode, beat, caption, sceneDurationFrames }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  // ── Window / body geometry (no separate title bar; the video fills it) ──
  const FW = 1920;
  const FH = 1080;
  const padTop = 48;
  const padSide = 80;
  const capBand = 150;
  const aspect = screencast.width / screencast.height;
  let bodyW = FW - 2 * padSide;
  let bodyH = bodyW / aspect;
  const availH = FH - padTop - capBand;
  if (bodyH > availH) {
    bodyH = availH;
    bodyW = bodyH * aspect;
  }

  // ── Video slice: play [tStart,tEnd] across this scene's frames ──
  const sliceSec = Math.max(0.1, beat.tEnd - beat.tStart);
  const sceneSec = Math.max(0.1, sceneDurationFrames / fps);
  const playbackRate = Math.min(3, Math.max(0.4, sliceSec / sceneSec));
  const startFrom = Math.round(beat.tStart * fps);
  const videoSrc = staticFile(`${demoId}/${screencast.video[themeMode]}`);

  // ── Zoom: full → target → full. Ease in over ~18f, hold, ease out over ~16f. ──
  const target = zoomTransform(beat.zoom, bodyW, bodyH);
  const inF = 18;
  const outF = 16;
  const easeIn = spring({ frame, fps, config: { damping: 200, mass: 0.7 }, durationInFrames: inF });
  const easeOut = spring({ frame: frame - (sceneDurationFrames - outF), fps, config: { damping: 200, mass: 0.7 }, durationInFrames: outF });
  // progress: 0 at the very start, 1 through the hold, back to 0 at the end.
  const p = beat.zoom ? easeIn * (1 - easeOut) : 0;
  const tr: Transform = { scale: lerp(1, target.scale, p), tx: lerp(0, target.tx, p), ty: lerp(0, target.ty, p) };

  const capOpacity = spring({ frame: frame - 4, fps, config: { damping: 200 } });

  return (
    <AbsoluteFill
      style={{ background: theme.bgGrad, fontFamily: theme.fontSans, padding: `${padTop}px ${padSide}px 0`, flexDirection: "column", alignItems: "center" }}
    >
      <div
        style={{
          position: "relative",
          width: bodyW,
          height: bodyH,
          borderRadius: 14,
          overflow: "hidden",
          background: theme.termBg,
          border: `1px solid ${theme.panelBorder}`,
          boxShadow: "0 40px 100px rgba(0,0,0,0.6)",
        }}
      >
        {/* Video + inline traffic lights share one zoom transform, so the lights
            sit on the app's top bar at full frame and zoom away with the content
            (rather than floating fixed over a zoomed-in view). */}
        <div
          style={{
            position: "absolute",
            top: 0,
            left: 0,
            width: bodyW,
            height: bodyH,
            transformOrigin: "0 0",
            transform: `translate(${tr.tx}px, ${tr.ty}px) scale(${tr.scale})`,
          }}
        >
          <OffthreadVideo src={videoSrc} startFrom={startFrom} playbackRate={playbackRate} muted style={{ position: "absolute", top: 0, left: 0, width: bodyW, height: bodyH }} />
          {/* Inline traffic lights over the app's own top bar (macOS unified title bar). */}
          <div style={{ position: "absolute", top: 17, left: 20, display: "flex", gap: 9 }}>
            <Light c="#ff5f57" />
            <Light c="#febc2e" />
            <Light c="#28c840" />
          </div>
        </div>
      </div>

      {/* caption lower-third (below the window, same as the terminal scenes) */}
      <div style={{ height: capBand, display: "flex", alignItems: "center", justifyContent: "center" }}>
        {caption ? (
          <div
            style={{
              maxWidth: 1500,
              padding: "16px 32px",
              background: "rgba(8,11,19,0.9)",
              border: "1px solid rgba(255,255,255,0.14)",
              borderRadius: 14,
              color: "#f4f7ff",
              fontSize: 29,
              lineHeight: 1.3,
              fontWeight: 500,
              textAlign: "center",
              opacity: capOpacity,
              transform: `translateY(${interpolate(capOpacity, [0, 1], [10, 0])}px)`,
            }}
          >
            {caption}
          </div>
        ) : null}
      </div>
    </AbsoluteFill>
  );
};
