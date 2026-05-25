// MDX-facing re-export of the kit's inline trigger.
//
// Imported via the package SUBPATH (not the index barrel) on purpose: the
// index re-exports KapiModal/KapiEmbed, which pull in xterm + the wasm boot
// path. Going straight to ./RunnableSnippet keeps the inline path SSR-clean and
// free of the heavy runtime — opening a docs page fetches zero wasm.
export { default } from "@neokapi/kapi-playground/RunnableSnippet";
export type { RunnableSnippetProps } from "@neokapi/kapi-playground/RunnableSnippet";
