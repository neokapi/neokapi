// Node types
export interface FlowNode {
  id: string;
  type: 'reader' | 'tool' | 'writer' | 'bridge-reader' | 'bridge-writer';
  name: string;
  label: string;
  bridge?: {
    filterClass: string;
    protocol: 'grpc' | 'netrpc';
    subprocess?: string;
  };
}

// Event types
export type EventType =
  | 'enter'
  | 'exit'
  | 'bridge-serialize'
  | 'bridge-deserialize'
  | 'bridge-send'
  | 'bridge-receive'
  | 'pool-acquire'
  | 'pool-release'
  | 'jvm-start'
  | 'jvm-ready'
  | 'grpc-open'
  | 'grpc-read-start'
  | 'grpc-read-end'
  | 'grpc-write-start'
  | 'grpc-write-end';

export interface TraceEvent {
  ts: number;
  type: EventType;
  nodeId: string;
  partId?: string;
  meta?: Record<string, unknown>;
}

export interface PartSnapshot {
  id: string;
  type: 'LayerStart' | 'LayerEnd' | 'Block' | 'Data' | 'Media' | 'GroupStart' | 'GroupEnd';
  summary: string;
  sourceText?: string;
  targetText?: string;
  detail?: unknown;
}

export interface PartSnapshotSet {
  initial: PartSnapshot;
  afterNode?: Record<string, PartSnapshot>;
}

export interface TraceFile {
  name: string;
  format?: string;
  preview: string;
}

export interface FlowTrace {
  name: string;
  description: string;
  nodes: FlowNode[];
  channelSize: number;
  events: TraceEvent[];
  parts: Record<string, PartSnapshotSet>;
  inputFile: TraceFile;
  outputFile: TraceFile;
  durationUs: number;
}

// Playback state
export interface PlaybackState {
  playing: boolean;
  time: number;
  speed: number;
  duration: number;
  eventIndex: number;
}

// Particle state for animation
export interface Particle {
  partId: string;
  partType: string;
  position: 'edge' | 'node';
  nodeId?: string;
  edgeIndex?: number;
  progress?: number;
  summary: string;
}
