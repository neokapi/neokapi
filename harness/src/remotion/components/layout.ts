/**
 * The frame grid every scene shares: where text may sit and where the picture
 * may sit, in a 1920x1080 frame. The docs embed the video at 800px, a 0.42
 * scale, so the sizes below are the sizes a reader gets on the page divided
 * by 0.42. A still of any scene at `--scale=0.42` is the check.
 */

export const FRAME_W = 1920;
export const FRAME_H = 1080;

/** Key text keeps this far from the sides, and from the top and bottom. */
export const SAFE_X = 142;
export const SAFE_Y = 178;

/** A picture (window, screenshot) keeps this far from the top and bottom. */
export const PIC_MARGIN_Y = 100;

/** The spoken-word captions in the lower third. */
export const CAPTION_FS = 46;
export const CAPTION_LH = 1.25;
export const CAPTION_PAD_Y = 18;
export const CAPTION_PAD_X = 34;
/** Height of one caption line with its pill. */
export const CAPTION_BAND = Math.round(CAPTION_FS * CAPTION_LH + 2 * CAPTION_PAD_Y);
/** Gap between a stacked window's bottom edge and the caption band. */
export const CAPTION_GAP = 20;

/** The chapter line above the window. */
export const CHAPTER_FS = 44;
export const CHAPTER_LH = 1.2;
export const CHAPTER_GAP = 16;
export const CHAPTER_BAND = Math.round(CHAPTER_FS * CHAPTER_LH + CHAPTER_GAP);

/** Terminal text. */
export const TERM_FS = 30;
export const TERM_LH = 1.4;
export const TERM_MAX_LINES = 16;
/** The transcript's side padding inside the window, and the width a line may take. */
export const TERM_PAD_X = 34;
export const TERM_TEXT_WIDTH = FRAME_W - 2 * SAFE_X - 2 * TERM_PAD_X;

/** Cards. */
export const TITLE_FS = 120;
export const SUBTITLE_FS = 52;
export const OUTRO_FS = 64;
export const POINTER_FS = 44;
export const PROMPT_FS = 52;

/** Frames of a fade or slide between scenes. */
export const TRANSITION_FRAMES = 12;

/** Frames the narration track fades in and out over. */
export const AUDIO_FADE_FRAMES = 12;

export interface SceneLayout {
  /** Left edge and width of the text and picture area. */
  left: number;
  width: number;
  /** Top of the chapter line, when there is one. */
  chapterTop: number;
  /** Where the picture may start and end vertically. */
  picTop: number;
  picBottom: number;
  /** Bottom edge of a window whose text the captions must never cover. */
  stackBottom: number;
  /** The caption band's edges. */
  captionTop: number;
  captionBottom: number;
}

/** The grid for a scene, with or without a chapter line above its picture. */
export function sceneLayout(hasChapter: boolean): SceneLayout {
  const captionBottom = FRAME_H - SAFE_Y;
  const captionTop = captionBottom - CAPTION_BAND;
  return {
    left: SAFE_X,
    width: FRAME_W - 2 * SAFE_X,
    chapterTop: SAFE_Y,
    picTop: hasChapter ? SAFE_Y + CHAPTER_BAND : PIC_MARGIN_Y,
    picBottom: FRAME_H - PIC_MARGIN_Y,
    stackBottom: captionTop - CAPTION_GAP,
    captionTop,
    captionBottom,
  };
}
