// Deep links into a lab.
//
// A lab's page is /learn/<lab-id>. Two query parameters say where in it and
// with what:
//
//   ?c=<chapter-id>   open at this chapter (the player replays the ones
//                     before it, silently, once the engine is up)
//   ?s=<token>        restore a shared session: the files a reader changed
//                     or added on top of the sample, so a colleague sees the
//                     same sandbox
//
// The token is base64url over UTF-8 JSON, like the playground's, and carries
// only what differs from the sample the lab seeds: the labs are small and a
// link has to stay pasteable. Leaf module: no React, no engine.

import type { LabFile } from "./curriculum/types.ts";

export const CHAPTER_PARAM = "c";
export const SESSION_PARAM = "s";

/** What a shared link restores on top of the sample: changed or added files, and removed ones. */
export interface SharedSession {
  files: LabFile[];
  removed: string[];
}

export interface LabLink {
  chapter?: string;
  session?: SharedSession;
}

const SESSION_VERSION = 1;

/** A link's session is refused above this many bytes of file content. */
export const SESSION_MAX_BYTES = 96 * 1024;

function toBase64Url(bytes: Uint8Array): string {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64Url(s: string): Uint8Array {
  const pad = s.length % 4 === 0 ? "" : "=".repeat(4 - (s.length % 4));
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/") + pad);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return bytes;
}

/** The bytes of content a session carries, to hold it under {@link SESSION_MAX_BYTES}. */
export function sessionBytes(session: SharedSession): number {
  const enc = new TextEncoder();
  return session.files.reduce((n, f) => n + enc.encode(f.content).length, 0);
}

/** Encode a session as a URL-safe token. Returns null when it is too large to share. */
export function serializeSession(session: SharedSession): string | null {
  if (sessionBytes(session) > SESSION_MAX_BYTES) return null;
  const payload = { v: SESSION_VERSION, f: session.files, r: session.removed };
  return toBase64Url(new TextEncoder().encode(JSON.stringify(payload)));
}

/** Decode a token; null for a malformed or unknown-version token. */
export function deserializeSession(token: string): SharedSession | null {
  try {
    const json = new TextDecoder().decode(fromBase64Url(token));
    const payload = JSON.parse(json) as { v?: number; f?: unknown; r?: unknown };
    if (payload.v !== SESSION_VERSION) return null;
    const files = Array.isArray(payload.f)
      ? payload.f.filter(
          (x): x is LabFile =>
            !!x &&
            typeof (x as LabFile).path === "string" &&
            typeof (x as LabFile).content === "string",
        )
      : [];
    const removed = Array.isArray(payload.r)
      ? payload.r.filter((x): x is string => typeof x === "string")
      : [];
    return { files, removed };
  } catch {
    return null;
  }
}

/** Read the lab link out of a query string (`location.search`, with or without the `?`). */
export function parseLabLink(search: string): LabLink {
  const params = new URLSearchParams(search.startsWith("?") ? search.slice(1) : search);
  const link: LabLink = {};
  const chapter = params.get(CHAPTER_PARAM);
  if (chapter && /^[a-z0-9-]+$/.test(chapter)) link.chapter = chapter;
  const token = params.get(SESSION_PARAM);
  if (token) {
    const session = deserializeSession(token);
    if (session) link.session = session;
  }
  return link;
}

/**
 * Write a lab link: the page's path plus the query for the chapter and,
 * when given, the session. `path` is the lab page without a query.
 */
export function formatLabLink(path: string, link: LabLink): string {
  const params = new URLSearchParams();
  if (link.chapter) params.set(CHAPTER_PARAM, link.chapter);
  if (link.session) {
    const token = serializeSession(link.session);
    if (token) params.set(SESSION_PARAM, token);
  }
  const query = params.toString();
  return query ? `${path}?${query}` : path;
}

/**
 * The files that differ from the seed: changed or added text files, and
 * seeded files that are gone. Binary files (ones that do not decode as UTF-8)
 * are left out; a lab's samples and outputs are text.
 */
export function diffAgainstSeed(
  seed: readonly LabFile[],
  current: readonly { path: string; bytes: Uint8Array }[],
): SharedSession {
  const dec = new TextDecoder("utf-8", { fatal: true });
  const seeded = new Map(seed.map((f) => [f.path, f.content]));
  const files: LabFile[] = [];
  const seen = new Set<string>();
  for (const f of current) {
    seen.add(f.path);
    let content: string;
    try {
      content = dec.decode(f.bytes);
    } catch {
      continue;
    }
    if (seeded.get(f.path) === content) continue;
    files.push({ path: f.path, content });
  }
  const removed = seed.filter((f) => !seen.has(f.path)).map((f) => f.path);
  return { files, removed };
}
