import MDXComponents from "@theme-original/MDXComponents";
import { Cli, Desktop } from "@site/src/components/surface/SurfaceBlock";

// Make <Cli> / <Desktop> available in every .md/.mdx page without an import, so a
// recipe can split CLI vs Kapi Desktop content that the navbar surface toggle
// shows/hides globally.
export default {
  ...MDXComponents,
  Cli,
  Desktop,
};
