// The map view's tile grid: each place with a centroid gets one square tile
// near where it lies. Tile area says nothing about geographic area; a small
// country and a large one take one tile each, so a count is never inflated by
// land mass.

export interface TileInput {
  id: string;
  /** `[lat, lon]`. */
  centroid: [number, number];
}

export interface Tile {
  id: string;
  col: number;
  row: number;
}

export interface TileGrid {
  tiles: Tile[];
  cols: number;
  rows: number;
}

function project([lat, lon]: [number, number], cosLat: number): [number, number] {
  return [lon * cosLat, -lat];
}

/**
 * Place every input on a grid near its centroid. Positions are snapped to a
 * grid whose cell is the median distance to a nearest neighbour (scaled by
 * `spread / 2.5`); a tile whose cell is taken moves to the free cell nearest
 * its exact position, searching outward ring by ring. Runs of empty rows and
 * columns then collapse to one. The result is deterministic for a given input
 * order.
 */
export function layoutTiles(inputs: readonly TileInput[], spread = 2.5): TileGrid {
  const valid = inputs.filter(
    (t) =>
      Array.isArray(t.centroid) && Number.isFinite(t.centroid[0]) && Number.isFinite(t.centroid[1]),
  );
  if (valid.length === 0) return { tiles: [], cols: 0, rows: 0 };
  const meanLat = valid.reduce((s, t) => s + t.centroid[0], 0) / valid.length;
  const cosLat = Math.max(0.35, Math.cos((meanLat * Math.PI) / 180));
  const pts = valid.map((t) => project(t.centroid, cosLat));
  const xs = pts.map((p) => p[0]);
  const ys = pts.map((p) => p[1]);
  const w = Math.max(...xs) - Math.min(...xs);
  const h = Math.max(...ys) - Math.min(...ys);
  // The cell is sized to the typical distance between neighbours, so a dense
  // region (Europe) keeps its shape; the ocean between regions is collapsed
  // afterwards. Without neighbours, the bounding box decides.
  const nearest = pts
    .map((p, i) =>
      Math.min(
        ...pts.map((q, j) =>
          i === j ? Number.POSITIVE_INFINITY : Math.hypot(p[0] - q[0], p[1] - q[1]),
        ),
      ),
    )
    .filter(Number.isFinite)
    .sort((a, b) => a - b);
  const median = nearest.length > 0 ? nearest[Math.floor(nearest.length / 2)] : 0;
  const area = Math.max(w * h, 1);
  const cell = Math.max(
    median > 0 ? median * (spread / 2.5) : Math.sqrt(area / (valid.length * spread)),
    0.5,
  );
  const x0 = Math.min(...xs);
  const y0 = Math.min(...ys);

  const taken = new Set<string>();
  const tiles: Tile[] = [];
  valid.forEach((t, i) => {
    const fx = (pts[i][0] - x0) / cell;
    const fy = (pts[i][1] - y0) / cell;
    let best: [number, number] | null = null;
    for (let ring = 0; best === null && ring < 64; ring++) {
      let bestD = Number.POSITIVE_INFINITY;
      const cx = Math.round(fx);
      const cy = Math.round(fy);
      for (let dx = -ring; dx <= ring; dx++) {
        for (let dy = -ring; dy <= ring; dy++) {
          if (Math.max(Math.abs(dx), Math.abs(dy)) !== ring) continue;
          const c = cx + dx;
          const r = cy + dy;
          if (taken.has(`${c},${r}`)) continue;
          const d = (c - fx) ** 2 + (r - fy) ** 2;
          if (d < bestD) {
            bestD = d;
            best = [c, r];
          }
        }
      }
    }
    const [c, r] = best ?? [Math.round(fx), Math.round(fy)];
    taken.add(`${c},${r}`);
    tiles.push({ id: t.id, col: c, row: r });
  });

  // Collapse each run of empty columns (and rows) to a single one: the order
  // and the separation between regions survive, the ocean does not.
  const compact = (values: number[]) => {
    const used = [...new Set(values)].sort((a, b) => a - b);
    const map = new Map<number, number>();
    let next = 0;
    used.forEach((v, i) => {
      if (i > 0 && v - used[i - 1] > 1) next += 1;
      map.set(v, next);
      next += 1;
    });
    return map;
  };
  const colMap = compact(tiles.map((t) => t.col));
  const rowMap = compact(tiles.map((t) => t.row));
  for (const t of tiles) {
    t.col = colMap.get(t.col) ?? 0;
    t.row = rowMap.get(t.row) ?? 0;
  }
  return {
    tiles,
    cols: Math.max(...tiles.map((t) => t.col)) + 1,
    rows: Math.max(...tiles.map((t) => t.row)) + 1,
  };
}

/** The places a geo axis puts on the map, and the groups each one sits in. */
export interface MapPlaces {
  /** Every country the map draws: the axis's own countries, then the members of its groups and regions. */
  places: TileInput[];
  /** Display names of the places that came only from a group's members. */
  names: Map<string, string>;
  /** For each place, the groups and regions of the axis that hold it. */
  within: Map<string, string[]>;
}

interface GeoValue {
  id: string;
  label?: string;
  centroid?: [number, number];
  members?: { id: string; label?: string; centroid: [number, number] }[];
}

/**
 * The places to draw for a geo axis. A country value is a tile of its own; a
 * group or region adds a tile for each country it holds, so a rule scoped to
 * the Nordics is drawn over Norway, Sweden and the rest, where it holds,
 * rather than only listed beside the map. A country that is both a value and
 * a member is drawn once.
 */
export function mapPlaces(values: readonly GeoValue[]): MapPlaces {
  const places: TileInput[] = [];
  const names = new Map<string, string>();
  const within = new Map<string, string[]>();
  const seen = new Set<string>();
  const add = (id: string, centroid: [number, number]) => {
    if (seen.has(id)) return;
    seen.add(id);
    places.push({ id, centroid });
  };
  for (const v of values) if (v.centroid) add(v.id, v.centroid);
  for (const v of values) {
    for (const m of v.members ?? []) {
      add(m.id, m.centroid);
      if (m.label && !names.has(m.id)) names.set(m.id, m.label);
      const groups = within.get(m.id) ?? [];
      if (!groups.includes(v.id)) groups.push(v.id);
      within.set(m.id, groups);
    }
  }
  return { places, names, within };
}
