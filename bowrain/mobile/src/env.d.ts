// Declarations for what Metro provides beyond TypeScript's view of the code.

// Stylesheets are compiled by Uniwind; importing one has only a side effect.
declare module "*.css";

// Metro's require.context: the modules in a directory, listed at bundle time.
interface MetroRequireContext {
  keys(): string[];
  (id: string): unknown;
}

declare namespace NodeJS {
  interface Require {
    context: (directory: string, deep?: boolean, filter?: RegExp) => MetroRequireContext;
  }
}
