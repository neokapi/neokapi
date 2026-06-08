// Side-effect → external-system mapping. A tool's declared sideEffects (and the
// redaction-secret it may produce) are surfaced as small "satellite" systems
// hanging off the node — the external things it reads from or writes to (TM,
// termbase, an API/provider, analytics, a redaction vault).

import { Database, BookMarked, Cloud, BarChart3, Vault, type LucideIcon } from "lucide-react";
import type { IOPort } from "./types";

export type SystemDirection = "read" | "write" | "both";

export interface SystemEffect {
  /** Stable key for React lists / de-duplication. */
  key: string;
  label: string;
  icon: LucideIcon;
  direction: SystemDirection;
  /** OKLCH accent. */
  color: string;
  description: string;
}

const TM_COLOR = "oklch(0.6 0.13 250)";
const TB_COLOR = "oklch(0.62 0.13 160)";
const API_COLOR = "oklch(0.64 0.15 300)";
const ANALYTICS_COLOR = "oklch(0.6 0.04 265)";
const VAULT_COLOR = "oklch(0.6 0.2 15)";

const BY_EFFECT: Record<string, SystemEffect> = {
  "tm-read": {
    key: "tm",
    label: "TM",
    icon: Database,
    direction: "read",
    color: TM_COLOR,
    description: "Reads from translation memory",
  },
  "tm-write": {
    key: "tm",
    label: "TM",
    icon: Database,
    direction: "write",
    color: TM_COLOR,
    description: "Writes to translation memory",
  },
  "termbase-read": {
    key: "termbase",
    label: "Termbase",
    icon: BookMarked,
    direction: "read",
    color: TB_COLOR,
    description: "Reads from the termbase",
  },
  "termbase-write": {
    key: "termbase",
    label: "Termbase",
    icon: BookMarked,
    direction: "write",
    color: TB_COLOR,
    description: "Writes to the termbase",
  },
  "api-call": {
    key: "api",
    label: "API",
    icon: Cloud,
    direction: "both",
    color: API_COLOR,
    description: "Calls an external API / provider",
  },
  analytics: {
    key: "analytics",
    label: "Analytics",
    icon: BarChart3,
    direction: "write",
    color: ANALYTICS_COLOR,
    description: "Emits analytics events",
  },
};

const VAULT: SystemEffect = {
  key: "vault",
  label: "Vault",
  icon: Vault,
  direction: "write",
  color: VAULT_COLOR,
  description: "Stores redaction secrets for later restore",
};

/**
 * Resolve the external systems a tool touches, from its declared side effects
 * plus the redaction-secret it may produce. Effects that map to the same system
 * with both read and write (e.g. tm-read + tm-write) collapse to a single
 * "both"-direction entry.
 */
export function getSystemEffects(sideEffects?: string[], produces?: IOPort[]): SystemEffect[] {
  const merged = new Map<string, SystemEffect>();
  for (const se of sideEffects ?? []) {
    const sys = BY_EFFECT[se];
    if (!sys) continue;
    const existing = merged.get(sys.key);
    if (existing && existing.direction !== sys.direction) {
      merged.set(sys.key, {
        ...existing,
        direction: "both",
        description: `${existing.label}: reads and writes`,
      });
    } else if (!existing) {
      merged.set(sys.key, sys);
    }
  }
  if ((produces ?? []).some((p) => p.type === "redaction.secret")) {
    merged.set(VAULT.key, VAULT);
  }
  return [...merged.values()];
}
