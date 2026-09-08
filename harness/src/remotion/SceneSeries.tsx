import React from "react";
import { Sequence, useCurrentFrame, useVideoConfig } from "remotion";
import { linearTiming } from "@remotion/transitions";
import type { Presentation } from "./scene-plan.ts";

export interface SceneSlot {
  key: string;
  name: string;
  from: number;
  durationInFrames: number;
  /** Frames this scene overlaps the previous one, and the next one. */
  transitionBefore: number;
  transitionAfter: number;
  presentationBefore?: Presentation;
  presentationAfter?: Presentation;
  children: React.ReactNode;
}

const noop = () => undefined;

/**
 * A scene inside its Sequence: plain for most of its life, wrapped in the
 * boundary's presentation while it enters and while it leaves. Later scenes
 * paint over earlier ones, so a fade is the entering scene fading in on top
 * and a slide moves both.
 */
const Transitioned: React.FC<{ slot: SceneSlot }> = ({ slot }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  let node: React.ReactNode = slot.children;
  const outStart = slot.durationInFrames - slot.transitionAfter;
  if (slot.presentationAfter && slot.transitionAfter > 0 && frame >= outStart) {
    const P = slot.presentationAfter.component;
    const progress = linearTiming({ durationInFrames: slot.transitionAfter }).getProgress({ frame: frame - outStart, fps });
    node = (
      <P
        presentationDirection="exiting"
        presentationProgress={progress}
        passedProps={slot.presentationAfter.props}
        presentationDurationInFrames={slot.transitionAfter}
        onElementImage={noop}
        onUnmount={noop}
        bothEnteringAndExiting={false}
      >
        {node}
      </P>
    );
  }
  if (slot.presentationBefore && slot.transitionBefore > 0 && frame < slot.transitionBefore) {
    const P = slot.presentationBefore.component;
    const progress = linearTiming({ durationInFrames: slot.transitionBefore }).getProgress({ frame, fps });
    node = (
      <P
        presentationDirection="entering"
        presentationProgress={progress}
        passedProps={slot.presentationBefore.props}
        presentationDurationInFrames={slot.transitionBefore}
        onElementImage={noop}
        onUnmount={noop}
        bothEnteringAndExiting={false}
      >
        {node}
      </P>
    );
  }
  return <>{node}</>;
};

/**
 * The scenes of a video in order, each premounted one second ahead so its
 * media has decoded before its first visible frame, with the transitions of
 * @remotion/transitions drawn across the overlaps the timeline planned.
 */
export const SceneSeries: React.FC<{ slots: SceneSlot[] }> = ({ slots }) => {
  const { fps } = useVideoConfig();
  return (
    <>
      {slots.map((slot) => (
        <Sequence key={slot.key} from={slot.from} durationInFrames={slot.durationInFrames} premountFor={fps} name={slot.name}>
          <Transitioned slot={slot} />
        </Sequence>
      ))}
    </>
  );
};
