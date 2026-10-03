/**
 * Round-trip between the change contract's edit text and the editor's
 * `(codedText, SpanInfo[])` shape.
 *
 * A read of the change service (kapi.change/v1) shows an edition's content as
 * edit text: plain characters, with each inline code an `<x id="…"/>` token
 * whose id says which code it is. `1` opens a paired code, `/1` closes it, `1/`
 * is a placeholder and `sub:1` a subblock reference; the read's `codes` map
 * says what each one is. A change operation takes the same text back
 * (`set_content` with `text`), and the service resolves every token against
 * the codes the edition holds, so what an editor sends keeps the codes it was
 * shown.
 *
 * The mapping is one-to-one: each token becomes one span, each span one token,
 * and the characters between them are kept as they are. No run sequence is
 * walked; the native form of a code never reaches the editor.
 */

import type { CodeRead } from "@neokapi/contract-types";

import { parseCodedSegments, segmentsToCodedText, type CodedSegment } from "./codedText";
import type { SpanInfo } from "../../types/span";

/** One `<x id="…"/>` token. */
const TOKEN = /<x id="([^"]+)"\/>/g;

/** The span type a subblock reference takes in the editor. */
const SUB_TYPE = "sub";

/**
 * The span a token stands for, from the id it shows and the code the read
 * lists under that id.
 */
function spanOf(token: string, codes: Readonly<Record<string, CodeRead>>): SpanInfo {
  if (token.startsWith("sub:")) {
    const id = token.slice(4);
    return { span_type: "placeholder", type: SUB_TYPE, id, data: "", equiv_text: id };
  }
  if (token.startsWith("/")) {
    const id = token.slice(1);
    const code = codes[id];
    return {
      span_type: "closing",
      type: code?.type ?? "",
      id,
      data: "",
      equiv_text: code?.equiv || id,
    };
  }
  if (token.endsWith("/")) {
    const id = token.slice(0, -1);
    const code = codes[token];
    return {
      span_type: "placeholder",
      type: code?.type ?? "",
      id,
      data: "",
      equiv_text: code?.equiv || id,
      ...(code?.disp ? { display_text: code.disp } : {}),
    };
  }
  const code = codes[token];
  return {
    span_type: "opening",
    type: code?.type ?? "",
    id: token,
    data: "",
    equiv_text: code?.equiv || token,
    ...(code?.disp ? { display_text: code.disp } : {}),
  };
}

/** The token a span stands for. */
function tokenOf(span: SpanInfo): string {
  if (span.span_type === "placeholder" && span.type === SUB_TYPE) return `<x id="sub:${span.id}"/>`;
  switch (span.span_type) {
    case "opening":
      return `<x id="${span.id}"/>`;
    case "closing":
      return `<x id="/${span.id}"/>`;
    default:
      return `<x id="${span.id}/"/>`;
  }
}

/**
 * The segments of an edit text: its characters, and a tag for each token,
 * typed from the read's codes. A token the codes do not list is still a tag,
 * so it survives the round trip and the service decides whether it holds.
 */
export function editTextToSegments(
  text: string,
  codes: Readonly<Record<string, CodeRead>> = {},
): CodedSegment[] {
  const segments: CodedSegment[] = [];
  let last = 0;
  for (const m of text.matchAll(TOKEN)) {
    const at = m.index ?? 0;
    if (at > last) segments.push({ type: "text", value: text.slice(last, at) });
    segments.push({ type: "tag", spanInfo: spanOf(m[1], codes) });
    last = at + m[0].length;
  }
  if (last < text.length) segments.push({ type: "text", value: text.slice(last) });
  return segments;
}

/** An edit text as the coded text and spans the inline-code editor takes. */
export function editTextToCoded(
  text: string,
  codes: Readonly<Record<string, CodeRead>> = {},
): { codedText: string; spans: SpanInfo[] } {
  return segmentsToCodedText(editTextToSegments(text, codes));
}

/** The edit text a change operation sends for coded text and its spans. */
export function codedToEditText(codedText: string, spans: SpanInfo[]): string {
  let out = "";
  for (const seg of parseCodedSegments(codedText, spans)) {
    out += seg.type === "text" ? seg.value : tokenOf(seg.spanInfo);
  }
  return out;
}
