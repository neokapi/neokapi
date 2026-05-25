// Ambient declaration for CSS module imports (`import styles from "./x.module.css"`).
declare module "*.module.css" {
  const classes: Record<string, string>;
  export default classes;
}

// Plain CSS side-effect imports (e.g. flow-editor pulls in @xyflow/react styles).
declare module "*.css";
