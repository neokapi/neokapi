import { z } from "zod";

const OverlaySchema = z.object({
  text: z.string(),
  position: z
    .enum(["top-left", "top-center", "top-right", "bottom-left", "bottom-center", "bottom-right"])
    .default("bottom-center"),
  startAt: z.number().default(0),
  duration: z.number().default(3),
  style: z.enum(["caption", "highlight", "step-number"]).default("caption"),
});

const FrameSchema = z.object({
  type: z.enum(["desktop", "terminal", "none"]).default("desktop"),
  title: z.string().optional(),
});

const TrimSchema = z.object({
  start: z.number().default(0),
  end: z.number().default(0),
});

const TitleCardScene = z.object({
  type: z.literal("title-card"),
  duration: z.number(),
  heading: z.string(),
  subheading: z.string().optional(),
  style: z.enum(["branded", "minimal", "dark", "light"]).default("branded"),
});

const TransitionScene = z.object({
  type: z.literal("transition"),
  effect: z.enum(["crossfade", "fade-black", "wipe-left"]).default("crossfade"),
  duration: z.number().default(0.5),
});

const RecordingScene = z.object({
  type: z.literal("recording"),
  source: z.string(),
  duration: z.union([z.literal("auto"), z.number()]).default("auto"),
  trim: TrimSchema.optional(),
  playbackRate: z.number().default(1.0),
  frame: FrameSchema.optional(),
  overlays: z.array(OverlaySchema).optional(),
});

const SceneSchema = z.discriminatedUnion("type", [TitleCardScene, TransitionScene, RecordingScene]);

const BrandingSchema = z.object({
  logo: z.string().default("logo.png"),
  primaryColor: z.string().default("#6366f1"),
  backgroundColor: z.string().default("#09090b"),
  fontFamily: z.string().default("Inter, system-ui, sans-serif"),
  cornerRadius: z.number().default(12),
});

const VideoSchema = z.object({
  id: z.string(),
  title: z.string(),
  width: z.number().default(1920),
  height: z.number().default(1080),
  fps: z.number().default(30),
  themes: z.array(z.string()).optional(),
});

export const ScriptSchema = z.object({
  video: VideoSchema,
  branding: BrandingSchema.default(() => ({
    logo: "logo.png",
    primaryColor: "#6366f1",
    backgroundColor: "#09090b",
    fontFamily: "Inter, system-ui, sans-serif",
    cornerRadius: 12,
  })),
  scenes: z.array(SceneSchema).min(1),
});

export type Script = z.infer<typeof ScriptSchema>;
export type Scene = z.infer<typeof SceneSchema>;
export type TitleCardSceneType = z.infer<typeof TitleCardScene>;
export type TransitionSceneType = z.infer<typeof TransitionScene>;
export type RecordingSceneType = z.infer<typeof RecordingScene>;
export type Overlay = z.infer<typeof OverlaySchema>;
export type Branding = z.infer<typeof BrandingSchema>;
export type Frame = z.infer<typeof FrameSchema>;

/** A resolved script has computed frame counts for all scenes. */
export interface ResolvedScene {
  scene: Scene;
  durationInFrames: number;
}

export interface ResolvedScript {
  video: Script["video"];
  branding: Script["branding"];
  scenes: ResolvedScene[];
  totalDurationInFrames: number;
}
