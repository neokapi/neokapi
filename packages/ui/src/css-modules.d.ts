// Ambient declaration for CSS module imports (`import styles from "./x.module.css"`)
// used by the preview kit (components/preview/*.module.css).
declare module "*.module.css" {
  const classes: Record<string, string>;
  export default classes;
}

// Plain CSS side-effect imports.
declare module "*.css";
