/**
 * Key marks: invisible delimiters that say which message a piece of rendered
 * text came from.
 *
 * A host showing a component to a reviewer (a review surface framing a
 * Storybook story) needs to point at the element that renders one message.
 * Searching the page for the message's text finds the wrong element when two
 * messages share words, misses a plural whose form differs from the catalog
 * text, and fails on a string the component splits across elements. With key
 * marking on (`setKeyMarking`), the runtime brackets every message it renders:
 *
 *   OPEN, the key spelled in Unicode tag characters, KEY_END,
 *   the message as it renders,
 *   CLOSE
 *
 * Every character used is default-ignorable: it takes no width and draws
 * nothing, so the component looks as it does without marks. A message key is
 * base62 (hashKey), and each of its characters maps to the tag character at
 * U+E0000 plus its ASCII code. Marks nest: a message rendered inside another
 * closes before the outer one does.
 *
 * The marks are a preview affordance. Nothing turns them on except a host
 * that asks (the Storybook decorator's host messages), and a shipped app
 * never renders them.
 */

/** Opens a marked message. U+2064 INVISIBLE PLUS. */
export const KEY_MARK_OPEN = "⁤";
/** Ends the key spelled after an open. U+E007F CANCEL TAG. */
export const KEY_MARK_KEY_END = "\u{E007F}";
/** Closes the innermost open message. U+2063 INVISIBLE SEPARATOR. */
export const KEY_MARK_CLOSE = "⁣";

const TAG_BASE = 0xe0000;

/** The open mark for a message key. */
export function keyMarkOpen(key: string): string {
  let tags = "";
  for (const ch of key) tags += String.fromCodePoint(TAG_BASE + (ch.codePointAt(0) ?? 0));
  return KEY_MARK_OPEN + tags + KEY_MARK_KEY_END;
}

/** A message located in marked text: its key and the span of what it rendered. */
export interface KeyMark {
  key: string;
  /** UTF-16 index of the first character of the rendered message. */
  start: number;
  /** UTF-16 index one past its last character. */
  end: number;
}

/**
 * The marked messages in a text, in the order they close. A host reading a
 * page walks its text the same way; this is the reference for that walk.
 */
export function readKeyMarks(text: string): KeyMark[] {
  const out: KeyMark[] = [];
  const open: { key: string; start: number }[] = [];
  let i = 0;
  while (i < text.length) {
    if (text[i] === KEY_MARK_OPEN) {
      let j = i + 1;
      let key = "";
      while (j < text.length) {
        const cp = text.codePointAt(j) ?? 0;
        if (cp === 0xe007f) {
          j += 2;
          break;
        }
        if (cp < TAG_BASE || cp > 0xe007e) break;
        key += String.fromCharCode(cp - TAG_BASE);
        j += 2;
      }
      open.push({ key, start: j });
      i = j;
      continue;
    }
    if (text[i] === KEY_MARK_CLOSE) {
      const top = open.pop();
      if (top) out.push({ key: top.key, start: top.start, end: i });
    }
    i++;
  }
  return out;
}

/** Text with every key mark removed. */
export function stripKeyMarks(text: string): string {
  return text.replace(/⁤(?:[\u{E0020}-\u{E007E}])*\u{E007F}|⁣/gu, "");
}
