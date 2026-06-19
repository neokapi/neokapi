// Diagram kit — uniform, theme-aware SVG diagrams for the docs.
//
// ArchitectureDiagram: the rich hero for the framework Architecture page.
// PipelineDiagram:      the reusable primitive for inline flow diagrams.
export { ArchitectureDiagram } from "./ArchitectureDiagram";
export type { ArchitectureDiagramProps } from "./ArchitectureDiagram";
export { PipelineDiagram } from "./PipelineDiagram";
export type { PipelineDiagramProps, PipelineStage, StageRole } from "./PipelineDiagram";
export { StreamDiagram } from "./StreamDiagram";
export type { StreamDiagramProps, StreamItem, StreamRole } from "./StreamDiagram";
export { RoundTripDiagram } from "./RoundTripDiagram";
export type { RoundTripDiagramProps } from "./RoundTripDiagram";
export { PhaseFlow } from "./PhaseFlow";
export type { PhaseFlowProps, PhaseNode } from "./PhaseFlow";
export { LanesDiagram } from "./LanesDiagram";
export type { LanesDiagramProps, LaneThread } from "./LanesDiagram";
export { SwimlaneDiagram } from "./SwimlaneDiagram";
export type { SwimlaneDiagramProps, SwimlaneActor, SwimlaneMessage } from "./SwimlaneDiagram";
export { RedactionDiagram } from "./RedactionDiagram";
export type { RedactionDiagramProps, RedactSpan } from "./RedactionDiagram";
export { AxisLadderDiagram } from "./AxisLadderDiagram";
export type { AxisLadderDiagramProps, AxisRung } from "./AxisLadderDiagram";
export { AxisFamiliesDiagram } from "./AxisFamiliesDiagram";
export type { AxisFamiliesDiagramProps, AxisFamily } from "./AxisFamiliesDiagram";
export { CycleDiagram } from "./CycleDiagram";
export type { CycleDiagramProps, CycleStep } from "./CycleDiagram";
