/**
 * Shared test fixtures for editor Storybook stories.
 * Realistic inline code data for HTML-in-JSON content.
 */
import type { SpanInfo } from "../../types/span";

// ---------------------------------------------------------------------------
// Spans (inline markup tags)
// ---------------------------------------------------------------------------

export const boldOpen: SpanInfo = { span_type: "opening", type: "fmt:bold", id: "1", data: "<b>" };
export const boldClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:bold",
  id: "1",
  data: "</b>",
};
export const italicOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:italic",
  id: "2",
  data: "<i>",
};
export const italicClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:italic",
  id: "2",
  data: "</i>",
};
export const linkOpen: SpanInfo = {
  span_type: "opening",
  type: "link:hyperlink",
  id: "3",
  data: '<a href="https://example.com">',
};
export const linkClose: SpanInfo = {
  span_type: "closing",
  type: "link:hyperlink",
  id: "3",
  data: "</a>",
};
export const codeOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:code",
  id: "4",
  data: "<code>",
};
export const codeClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:code",
  id: "4",
  data: "</code>",
};
export const lineBreak: SpanInfo = {
  span_type: "placeholder",
  type: "struct:break",
  id: "5",
  data: "<br/>",
};

// Unicode markers used in coded text
const O = "\uE001"; // opening
const C = "\uE002"; // closing
const P = "\uE003"; // placeholder

// ---------------------------------------------------------------------------
// Coded text samples
// ---------------------------------------------------------------------------

/** "Click <b>here</b> to continue" */
export const simpleBoldCodedText = `Click ${O}here${C} to continue`;
export const simpleBoldSpans: SpanInfo[] = [boldOpen, boldClose];

/** "Visit <a>our website</a> for <i>more info</i>" */
export const linkAndItalicCodedText = `Visit ${O}our website${C} for ${O}more info${C}`;
export const linkAndItalicSpans: SpanInfo[] = [linkOpen, linkClose, italicOpen, italicClose];

/** All tag types in one segment */
export const richCodedText = `${O}Bold${C} and ${O}italic${C} with ${O}a link${C} plus ${O}code${C} and ${P}`;
export const richSpans: SpanInfo[] = [
  boldOpen,
  boldClose,
  italicOpen,
  italicClose,
  linkOpen,
  linkClose,
  codeOpen,
  codeClose,
  lineBreak,
];
