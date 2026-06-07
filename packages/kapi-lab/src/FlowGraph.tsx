import React from "react";
import type { FlowNode, Particle } from "@neokapi/ui-primitives/preview";
import styles from "./styles.module.css";

interface FlowGraphProps {
  nodes: FlowNode[];
  channelSize: number;
  particles: Particle[];
  channelFills: Record<string, number>;
  activeNodes: Set<string>;
  selectedPartId: string | null;
  onPartClick: (partId: string) => void;
}

const NODE_WIDTH = 160;
const NODE_HEIGHT = 80;
const WORKER_LANE_HEIGHT = 24;
const BRIDGE_WIDTH = 240;
const NODE_GAP = 120;
const PADDING_X = 60;
const PADDING_Y = 60;
// Vertical gap between particles that share a node/edge, so co-located parts
// fan out into distinct dots instead of stacking into one.
const PARTICLE_SPREAD = 13;

const PART_COLORS: Record<string, string> = {
  Block: "#3b82f6",
  LayerStart: "#22c55e",
  LayerEnd: "#22c55e",
  Data: "#94a3b8",
  Media: "#f59e0b",
  GroupStart: "#a855f7",
  GroupEnd: "#a855f7",
};

const NODE_COLORS: Record<string, { fill: string; stroke: string }> = {
  reader: { fill: "rgba(34, 197, 94, 0.12)", stroke: "#22c55e" },
  tool: { fill: "rgba(100, 116, 139, 0.12)", stroke: "#64748b" },
  writer: { fill: "rgba(59, 130, 246, 0.12)", stroke: "#3b82f6" },
  "bridge-reader": { fill: "rgba(34, 197, 94, 0.12)", stroke: "#22c55e" },
  "bridge-writer": { fill: "rgba(59, 130, 246, 0.12)", stroke: "#3b82f6" },
};

function getNodeWidth(node: FlowNode): number {
  return node.bridge ? BRIDGE_WIDTH : NODE_WIDTH;
}

function getNodeHeight(node: FlowNode): number {
  const c = node.concurrency ?? 0;
  if (c > 1) return NODE_HEIGHT + (c - 1) * WORKER_LANE_HEIGHT;
  return NODE_HEIGHT;
}

function getMaxNodeHeight(nodes: FlowNode[]): number {
  return Math.max(NODE_HEIGHT, ...nodes.map(getNodeHeight));
}

function getNodeX(nodes: FlowNode[], index: number): number {
  let x = PADDING_X;
  for (let i = 0; i < index; i++) x += getNodeWidth(nodes[i]) + NODE_GAP;
  return x;
}

function getNodeCenter(
  nodes: FlowNode[],
  index: number,
  maxHeight: number,
): { x: number; y: number } {
  const x = getNodeX(nodes, index) + getNodeWidth(nodes[index]) / 2;
  const y = PADDING_Y + maxHeight / 2;
  return { x, y };
}

function getTotalWidth(nodes: FlowNode[]): number {
  if (nodes.length === 0) return PADDING_X * 2;
  let w = PADDING_X;
  for (let i = 0; i < nodes.length; i++) {
    w += getNodeWidth(nodes[i]);
    if (i < nodes.length - 1) w += NODE_GAP;
  }
  return w + PADDING_X;
}

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

export default function FlowGraph({
  nodes,
  channelSize,
  particles,
  channelFills,
  activeNodes,
  selectedPartId,
  onPartClick,
}: FlowGraphProps): React.ReactElement {
  const maxNodeHeight = getMaxNodeHeight(nodes);
  const totalWidth = getTotalWidth(nodes);
  const totalHeight = PADDING_Y * 2 + maxNodeHeight;

  const nodeIndexMap = new Map<string, number>();
  nodes.forEach((n, i) => nodeIndexMap.set(n.id, i));

  // Fan co-located particles apart: group by slot (a node — optionally a worker
  // lane — or an edge), then give each a vertical offset by its index within the
  // slot so all of them are visible rather than overlapping at one point. The
  // gap shrinks as a slot gets crowded so a large batch still fits.
  const slotKeyOf = (p: Particle): string =>
    p.position === "node" ? `n:${p.nodeId}:${p.worker ?? "_"}` : `e:${p.edgeIndex}`;
  const slotCounts = new Map<string, number>();
  for (const p of particles) {
    const k = slotKeyOf(p);
    slotCounts.set(k, (slotCounts.get(k) ?? 0) + 1);
  }
  const slotSeen = new Map<string, number>();
  const fanOffsets = particles.map((p) => {
    const k = slotKeyOf(p);
    const count = slotCounts.get(k) ?? 1;
    const idx = slotSeen.get(k) ?? 0;
    slotSeen.set(k, idx + 1);
    if (count <= 1) return 0;
    const gap = Math.min(PARTICLE_SPREAD, maxNodeHeight / count);
    return (idx - (count - 1) / 2) * gap;
  });

  return (
    <svg
      width={totalWidth}
      height={totalHeight + 30}
      viewBox={`0 0 ${totalWidth} ${totalHeight + 30}`}
      className={styles.svgGraph}
    >
      <defs>
        <marker id="arrowhead" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
          <polygon points="0 0, 8 3, 0 6" className={styles.arrowhead} />
        </marker>
      </defs>

      {/* Edges */}
      {nodes.map((node, i) => {
        if (i >= nodes.length - 1) return null;
        const fromCenter = getNodeCenter(nodes, i, maxNodeHeight);
        const toCenter = getNodeCenter(nodes, i + 1, maxNodeHeight);
        const fromX = fromCenter.x + getNodeWidth(node) / 2;
        const toX = toCenter.x - getNodeWidth(nodes[i + 1]) / 2;
        const y = fromCenter.y;

        const cpOffset = (toX - fromX) * 0.3;
        const pathD = `M ${fromX} ${y} C ${fromX + cpOffset} ${y}, ${toX - cpOffset} ${y}, ${toX} ${y}`;

        const edgeKey = `${node.id}->${nodes[i + 1].id}`;
        const fill = channelFills[edgeKey] || 0;
        const fillRatio = Math.min(1, fill / Math.max(1, channelSize));

        const meterX = (fromX + toX) / 2 - 30;
        const meterY = y + maxNodeHeight / 2 + 8;
        const meterWidth = 60;
        const meterHeight = 6;

        return (
          <g key={`edge-${i}`}>
            <path d={pathD} className={styles.edgePath} markerEnd="url(#arrowhead)" />
            <rect
              x={meterX}
              y={meterY}
              width={meterWidth}
              height={meterHeight}
              rx={3}
              ry={3}
              className={styles.bufferMeterBg}
            />
            <rect
              x={meterX}
              y={meterY}
              width={meterWidth * fillRatio}
              height={meterHeight}
              rx={3}
              ry={3}
              className={styles.bufferMeterFill}
            />
            {fill > 0 && (
              <text
                x={meterX + meterWidth + 6}
                y={meterY + meterHeight / 2 + 1}
                className={styles.bufferLabel}
              >
                {fill}
              </text>
            )}
          </g>
        );
      })}

      {/* Nodes */}
      {nodes.map((node, i) => {
        const x = getNodeX(nodes, i);
        const h = getNodeHeight(node);
        const y = PADDING_Y + (maxNodeHeight - h) / 2;
        const w = getNodeWidth(node);
        const isActive = activeNodes.has(node.id);
        const colors = NODE_COLORS[node.type] || NODE_COLORS.tool;
        const concurrency = node.concurrency ?? 0;

        return (
          <g key={node.id} className={isActive ? styles.nodeActive : undefined}>
            <rect
              x={x}
              y={y}
              width={w}
              height={h}
              fill={colors.fill}
              stroke={colors.stroke}
              className={styles.nodeRect}
            />
            {node.bridge && (
              <rect
                x={x + 12}
                y={y + 12}
                width={w - 24}
                height={h - 24}
                fill="none"
                stroke={colors.stroke}
                strokeDasharray="4 3"
                className={styles.nodeRect}
                opacity={0.6}
              />
            )}
            <text x={x + w / 2} y={y + 18} className={styles.nodeTypeLabel}>
              {node.type.replace("bridge-", "bridge ")}
            </text>
            <text x={x + w / 2} y={y + 38} className={styles.nodeLabel}>
              {node.label}
            </text>
            {node.bridge && (
              <text x={x + w / 2} y={y + h - 10} className={styles.bridgeLabel}>
                {node.bridge.filterClass}
              </text>
            )}
            {concurrency > 1 && (
              <g>
                {Array.from({ length: concurrency }, (_, wi) => {
                  const laneY = y + 52 + wi * WORKER_LANE_HEIGHT;
                  const laneW = w - 20;
                  const laneH = WORKER_LANE_HEIGHT - 4;
                  const laneActive = particles.some(
                    (p) => p.position === "node" && p.nodeId === node.id && p.worker === wi,
                  );
                  return (
                    <g key={`lane-${wi}`}>
                      <rect
                        x={x + 10}
                        y={laneY}
                        width={laneW}
                        height={laneH}
                        rx={4}
                        ry={4}
                        fill={laneActive ? colors.stroke : "transparent"}
                        opacity={laneActive ? 0.15 : 1}
                        stroke={colors.stroke}
                        strokeWidth={1}
                        strokeDasharray={laneActive ? "none" : "3 2"}
                        strokeOpacity={0.4}
                      />
                      <text x={x + 18} y={laneY + laneH / 2 + 1} className={styles.workerLabel}>
                        w{wi}
                      </text>
                    </g>
                  );
                })}
              </g>
            )}
          </g>
        );
      })}

      {/* Particles */}
      {particles.map((particle, pIndex) => {
        let cx: number;
        let cy: number;

        if (particle.position === "node" && particle.nodeId) {
          const nIdx = nodeIndexMap.get(particle.nodeId);
          if (nIdx === undefined) return null;
          const node = nodes[nIdx];
          const center = getNodeCenter(nodes, nIdx, maxNodeHeight);
          cx = center.x;
          cy = center.y;
          const concurrency = node.concurrency ?? 0;
          if (concurrency > 1 && particle.worker !== undefined) {
            const h = getNodeHeight(node);
            const nodeY = PADDING_Y + (maxNodeHeight - h) / 2;
            const laneY = nodeY + 52 + particle.worker * WORKER_LANE_HEIGHT;
            const laneH = WORKER_LANE_HEIGHT - 4;
            cy = laneY + laneH / 2;
            cx = center.x + 16;
          }
        } else if (particle.position === "edge" && particle.edgeIndex !== undefined) {
          const i = particle.edgeIndex;
          if (i >= nodes.length - 1) return null;
          const fromCenter = getNodeCenter(nodes, i, maxNodeHeight);
          const toCenter = getNodeCenter(nodes, i + 1, maxNodeHeight);
          const fromX = fromCenter.x + getNodeWidth(nodes[i]) / 2;
          const toX = toCenter.x - getNodeWidth(nodes[i + 1]) / 2;
          const progress = particle.progress ?? 0.5;
          cx = lerp(fromX, toX, progress);
          cy = fromCenter.y;
        } else {
          return null;
        }

        // Spread out parts sharing this slot so they don't stack into one dot.
        cy += fanOffsets[pIndex];

        const color = PART_COLORS[particle.partType] || "#94a3b8";
        const isSelected = particle.partId === selectedPartId;
        const radius = isSelected ? 9 : 7;

        return (
          <g
            key={particle.partId}
            onClick={() => onPartClick(particle.partId)}
            style={{ cursor: "pointer" }}
          >
            {isSelected && (
              <circle
                cx={cx}
                cy={cy}
                r={radius + 4}
                fill="none"
                stroke={color}
                strokeWidth={2}
                opacity={0.5}
                className={`${styles.particle} ${styles.particleSelected}`}
              />
            )}
            <circle
              cx={cx}
              cy={cy}
              r={radius}
              fill={color}
              stroke="white"
              strokeWidth={1.5}
              className={styles.particle}
            />
            <title>{particle.summary}</title>
          </g>
        );
      })}
    </svg>
  );
}
