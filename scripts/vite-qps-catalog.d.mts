// Types for scripts/vite-qps-catalog.mjs, which the app vite configs and the
// Storybook configs import.

/** Regenerate one surface's qps dictionary with `make <target>`. Returns whether it ran. */
export function generateQpsCatalog(target: string, warn?: (message: string) => void): boolean;

/** A Vite plugin that runs `make <target>` once at build start. */
export default function qpsCatalog(target: string): {
  name: string;
  buildStart(this: { warn(message: string): void }): void;
};
