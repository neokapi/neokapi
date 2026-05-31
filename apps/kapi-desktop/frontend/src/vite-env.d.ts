/// <reference types="vite/client" />

// CSS-only font packages (@fontsource-variable/*) are imported for their side
// effects and ship no type declarations; declare them so the type checker
// (`vp check` / tsc) accepts the side-effect imports. `*.css` and
// `import.meta.env` are already covered by the vite/client reference above.
declare module "@fontsource-variable/*";
