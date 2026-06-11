// The single point where the generated Wails bindings enter typed code.
// The bindings are plain .js files generated outside the TS project root
// (vite.config.ts ignores bindings/**), so this is the one sanctioned
// ts-ignore for them — import { Backend } from "../api/backend" everywhere
// else instead of repeating the suppression.
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
export * as Backend from "../../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js";
