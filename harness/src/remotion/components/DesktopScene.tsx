import React from "react";
import { AbsoluteFill, Freeze, Sequence, staticFile, useCurrentFrame, useVideoConfig } from "remotion";
import { Audio, Video } from "@remotion/media";
import { mouseClick } from "@remotion/sfx";
import type { Screencast, ScreencastBeat, ZoomRect } from "../../types.ts";
import { theme } from "./theme.ts";
import type { ThemeMode } from "./theme.ts";
import type { SceneLayout } from "./layout.ts";
import { BoxMark } from "./Marks.tsx";
import { clickFrames, planPlayback } from "../playback.ts";
import { cardGeometry, isWebDemo, MAX_CROP_SCALE } from "../../lib/crop.ts";
import { cameraAt, zoomPose } from "../camera.ts";

/**
 * The macOS window buttons, in the recorded page's own CSS pixels: 14px across,
 * centres 23px apart, the first centred 25px in from the window's top-left
 * corner, measured off a Kapi Desktop window. `s` scales them with the picture.
 */
/** A window button in the browser bar the web demos are framed under. */
const BarLight: React.FC<{ c: string }> = ({ c }) => (
  <span style={{ width: 13, height: 13, borderRadius: 13, background: c, display: "inline-block", boxShadow: "0 0 0 0.5px rgba(0,0,0,0.18)" }} />
);

const LIGHT = { size: 14, first: 25, step: 23 };
const Light: React.FC<{ c: string; i: number; s: number }> = ({ c, i, s }) => {
  const d = LIGHT.size * s;
  return (
    <span
      style={{
        position: "absolute",
        left: (LIGHT.first + i * LIGHT.step) * s - d / 2,
        top: LIGHT.first * s - d / 2,
        width: d,
        height: d,
        borderRadius: d,
        background: c,
        boxShadow: "0 0 0 0.5px rgba(0,0,0,0.18)",
      }}
    />
  );
};


export { MAX_CROP_SCALE };

/**
 * One desktop walkthrough beat, presented as a window on a frosted backdrop.
 * The camera moves as `cameraAt` says: wide, in on the beat's region, and out
 * again. The picture plays its slice at a natural rate and holds its last
 * frame when the narration is longer.
 */
export const DesktopScene: React.FC<{
  demoId: string;
  screencast: Screencast;
  themeMode: ThemeMode;
  beat: ScreencastBeat;
  /** The previous beat's data, so the camera can ease from its shot. */
  prevBeat: ScreencastBeat | null;
  /** The next beat's data, so the camera can stay zoomed into a nearby shot. */
  nextBeat: ScreencastBeat | null;
  sceneDurationFrames: number;
  layout: SceneLayout;
  /** An authored crop box, over the recorded one. */
  crop?: ZoomRect;
  /** An authored multiplier on the crop's fitted scale. */
  zoom?: number;
  /** An authored box to draw around, over the recorded one. */
  highlight?: ZoomRect;
}> = ({ demoId, screencast, themeMode, beat, prevBeat, nextBeat, sceneDurationFrames, layout, crop, zoom, highlight }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  // ── Window / body geometry (fit the screencast aspect into the picture box) ──
  // Web demos capture a browser SPA with no native window chrome, so they are
  // framed inside a browser top bar (traffic lights + an address pill) and the
  // capture sits BELOW it. Native-app demos keep their own title-bar gutter,
  // so they get the lightweight dots overlay instead.
  // Everything below is in the picture box's own coordinates: the box clips
  // the pushed-in card, so a crop never covers the chapter line above it.
  const isWeb = isWebDemo(demoId);
  const boxW = layout.width;
  const boxH = layout.picBottom - layout.picTop;
  const geo = cardGeometry(layout, screencast.width / screencast.height, isWeb);
  const { bw, bh, bar, cardW, cardH } = geo;
  const cardLeft = (boxW - cardW) / 2;
  const cardTop = (boxH - cardH) / 2;

  // ── Playback: the slice at a natural rate, or at 1x with the last frame held ──
  const sliceSec = Math.max(0.1, beat.tEnd - beat.tStart);
  const plan = planPlayback(sliceSec, sceneDurationFrames, fps);
  const trimBefore = Math.round(beat.tStart * fps);
  const videoSrc = staticFile(`${demoId}/${screencast.video[themeMode]}`);
  const held = plan.heldFrames > 0;
  const clicks = clickFrames(screencast.clicks?.[themeMode], beat, plan, fps);

  // ── Camera: wide, ease in on the region, hold, ease out ──
  const shot = zoomPose(crop ?? beat.zoom, zoom ?? 1, geo);
  const readyFrame = beat.zoomAt ? Math.min(plan.playFrames, Math.round(((beat.zoomAt - beat.tStart) / plan.rate) * fps)) : 0;
  const cam = cameraAt(frame, sceneDurationFrames, fps, shot, zoomPose(prevBeat?.zoom, 1, geo), zoomPose(nextBeat?.zoom, 1, geo), geo, readyFrame);

  const mark = highlight ?? beat.highlight ?? null;

  return (
    <AbsoluteFill style={{ background: theme.bgGrad, fontFamily: theme.fontSans }}>
      {/* Frosted backdrop: a blurred, dimmed copy of the same frame fills the
          whole frame behind the card, so an angled or pushed-in window never
          leaves a flat empty void beside it. */}
      <AbsoluteFill style={{ overflow: "hidden" }}>
        <Freeze frame={plan.playFrames - 1} active={held ? (f) => f >= plan.playFrames : false}>
          <Video
            src={videoSrc}
            trimBefore={trimBefore}
            playbackRate={plan.rate}
            muted
            objectFit="cover"
            name="backdrop"
            style={{
              position: "absolute",
              inset: 0,
              width: "100%",
              height: "100%",
              scale: "1.3",
              filter: "blur(40px) saturate(0.9) brightness(0.66)",
            }}
          />
        </Freeze>
        <AbsoluteFill style={{ background: "radial-gradient(130% 130% at 50% 40%, rgba(8,11,18,0.16), rgba(4,6,12,0.68))" }} />
      </AbsoluteFill>

      {/* The picture box: a 3D canvas in which the window is a card and the
          transform is the camera move; the box clips whatever a crop pushes
          past its edges. */}
      <div
        style={{
          position: "absolute",
          left: layout.left,
          top: layout.picTop,
          width: boxW,
          height: boxH,
          overflow: "hidden",
        }}
      >
        <div
          style={{
            position: "absolute",
            left: cardLeft,
            top: cardTop,
            width: cardW,
            height: cardH,
            transformOrigin: "center center",
            transform: `translate(${cam.tx}px, ${cam.ty}px) scale(${cam.s})`,
            borderRadius: 14,
            overflow: "hidden",
            background: theme.termBg,
            border: `1px solid ${theme.panelBorder}`,
            boxShadow: "0 50px 120px rgba(0,0,0,0.65), 0 8px 28px rgba(0,0,0,0.45)",
          }}
        >
          {isWeb ? (
            // Browser chrome: traffic lights + an address pill, with the captured
            // web app placed BELOW it so the dots never overlap app content.
            <div
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: cardW,
                height: bar,
                display: "flex",
                alignItems: "center",
                background: theme.chrome,
                borderBottom: `1px solid ${theme.panelBorder}`,
              }}
            >
              <div style={{ display: "flex", gap: 9, marginLeft: 20 }}>
                <BarLight c="#ff5f57" />
                <BarLight c="#febc2e" />
                <BarLight c="#28c840" />
              </div>
              <div
                style={{
                  flex: 1,
                  margin: "0 22px 0 20px",
                  height: 24,
                  borderRadius: 12,
                  background: theme.resultBg,
                  border: `1px solid ${theme.panelBorder}`,
                  display: "flex",
                  alignItems: "center",
                  paddingLeft: 14,
                  color: theme.dim,
                  fontSize: 13,
                  fontFamily: theme.fontSans,
                }}
              >
                bowrain.cloud
              </div>
            </div>
          ) : null}
          <Freeze frame={plan.playFrames - 1} active={held ? (f) => f >= plan.playFrames : false}>
            <Video
              src={videoSrc}
              trimBefore={trimBefore}
              playbackRate={plan.rate}
              muted
              objectFit="fill"
              name={`beat:${beat.id}`}
              style={{ position: "absolute", top: bar, left: 0, width: bw, height: bh }}
            />
          </Freeze>
          {mark ? <BoxMark left={mark.x * bw} top={bar + mark.y * bh} width={mark.w * bw} height={mark.h * bh} /> : null}
          {!isWeb && (
            // Native-app demos: the macOS window buttons, drawn into the
            // title-bar strip the app reserves when it runs on macOS
            // (the recorder reports darwin), scaled with the picture.
            <>
              <Light c="#ff5f57" i={0} s={bw / screencast.width} />
              <Light c="#febc2e" i={1} s={bw / screencast.width} />
              <Light c="#28c840" i={2} s={bw / screencast.width} />
            </>
          )}
        </div>
      </div>

      {/* The recorded clicks, as the sound of a click where the ripple blooms. */}
      {clicks.map((f, i) => (
        <Sequence key={`click-${i}`} from={f} layout="none" name="click">
          <Audio src={mouseClick} volume={0.45} name="click" />
        </Sequence>
      ))}
    </AbsoluteFill>
  );
};
