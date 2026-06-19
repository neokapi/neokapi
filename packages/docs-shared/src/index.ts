export { default as ThemedVideo } from "./ThemedVideo";
export { default as ThemedImage } from "./ThemedImage";
export { KapiLink, BowrainLink } from "./CrossSiteLink";
export { readCdnConfig, cdnEnabled, cdnHref } from "./cdn";
export type { CdnConfig } from "./cdn";

// Diagram kit — uniform, theme-aware SVG diagrams shared by both docs sites.
// Each component imports its own `diagram.css`, so the styles ship with the
// component (no separate CSS import needed at the consumer).
export {
  ArchitectureDiagram,
  PipelineDiagram,
  StreamDiagram,
  RoundTripDiagram,
  PhaseFlow,
  LanesDiagram,
  SwimlaneDiagram,
  RedactionDiagram,
  AxisLadderDiagram,
  AxisFamiliesDiagram,
  CycleDiagram,
} from "./diagram";
export type {
  ArchitectureDiagramProps,
  PipelineDiagramProps,
  PipelineStage,
  StageRole,
  StreamDiagramProps,
  StreamItem,
  StreamRole,
  RoundTripDiagramProps,
  PhaseFlowProps,
  PhaseNode,
  LanesDiagramProps,
  LaneThread,
  SwimlaneDiagramProps,
  SwimlaneActor,
  SwimlaneMessage,
  RedactionDiagramProps,
  RedactSpan,
  AxisLadderDiagramProps,
  AxisRung,
  AxisFamiliesDiagramProps,
  AxisFamily,
  CycleDiagramProps,
  CycleStep,
} from "./diagram";
