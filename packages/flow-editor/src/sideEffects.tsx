// Side-effect → external-system mapping. A tool's declared sideEffects (and the
// redaction-secret it may produce) are surfaced as small "satellite" systems
// hanging off the node — the external things it reads from or writes to (content memory,
// terms, an API/provider, analytics, a redaction vault).

import { t } from "@neokapi/i18n-react/runtime";
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

const MEMORY_COLOR = "oklch(0.6 0.13 250)";
const TB_COLOR = "oklch(0.62 0.13 160)";
const API_COLOR = "oklch(0.64 0.15 300)";
const ANALYTICS_COLOR = "oklch(0.6 0.04 265)";
const VAULT_COLOR = "oklch(0.6 0.2 15)";

// The KEYS below are wire values emitted by Go (`core/schema/annotations.go`,
// `SideEffect`), not display vocabulary. They keep their historical `tm-` and
// `termbase-` spellings and must match that Go list exactly — a mismatch does
// not fail a build, it just silently stops the badge rendering. Rename the
// labels and descriptions freely; leave the keys alone.
const BY_EFFECT: Record<string, SystemEffect> = {
  "tm-read": {
    key: "tm",
    get label() {
      return t("Memory", "external system");
    },
    icon: Database,
    direction: "read",
    color: MEMORY_COLOR,
    get description() {
      return t("Reads from content memory");
    },
  },
  "tm-write": {
    key: "tm",
    get label() {
      return t("Memory", "external system");
    },
    icon: Database,
    direction: "write",
    color: MEMORY_COLOR,
    get description() {
      return t("Writes to content memory");
    },
  },
  "termbase-read": {
    key: "terms",
    get label() {
      return t("Terms", "external system");
    },
    icon: BookMarked,
    direction: "read",
    color: TB_COLOR,
    get description() {
      return t("Reads from the terms store");
    },
  },
  "termbase-write": {
    key: "terms",
    get label() {
      return t("Terms", "external system");
    },
    icon: BookMarked,
    direction: "write",
    color: TB_COLOR,
    get description() {
      return t("Writes to the terms store");
    },
  },
  "api-call": {
    key: "api",
    get label() {
      return t("API", "external system");
    },
    icon: Cloud,
    direction: "both",
    color: API_COLOR,
    get description() {
      return t("Calls an external API / provider");
    },
  },
  analytics: {
    key: "analytics",
    get label() {
      return t("Analytics", "external system");
    },
    icon: BarChart3,
    direction: "write",
    color: ANALYTICS_COLOR,
    get description() {
      return t("Emits analytics events");
    },
  },
};

const VAULT_KEY = "vault";

// A tool that produces a redaction.secret writes it to the vault (redact); one
// that consumes a redaction.secret reads it back to restore originals
// (unredact). A tool doing both collapses to a single "both"-direction entry.
const VAULT_WRITE: SystemEffect = {
  key: VAULT_KEY,
  get label() {
    return t("Vault", "external system");
  },
  icon: Vault,
  direction: "write",
  color: VAULT_COLOR,
  get description() {
    return t("Stores redaction secrets for later restore");
  },
};

const VAULT_READ: SystemEffect = {
  key: VAULT_KEY,
  get label() {
    return t("Vault", "external system");
  },
  icon: Vault,
  direction: "read",
  color: VAULT_COLOR,
  get description() {
    return t("Reads redaction secrets to restore originals");
  },
};

/**
 * Resolve the external systems a tool touches, from its declared side effects
 * plus the redaction-secret it produces (writes to the vault) or consumes
 * (reads from the vault). Effects that map to the same system with both read
 * and write (e.g. tm-read + tm-write) collapse to a single "both"-direction
 * entry.
 */
export function getSystemEffects(
  sideEffects?: string[],
  produces?: IOPort[],
  consumes?: IOPort[],
): SystemEffect[] {
  const merged = new Map<string, SystemEffect>();
  for (const se of sideEffects ?? []) {
    const sys = BY_EFFECT[se];
    if (!sys) continue;
    const existing = merged.get(sys.key);
    if (existing && existing.direction !== sys.direction) {
      merged.set(sys.key, {
        ...existing,
        direction: "both",
        description: t("{label}: reads and writes", { label: existing.label }),
      });
    } else if (!existing) {
      merged.set(sys.key, sys);
    }
  }
  const writesSecret = (produces ?? []).some((p) => p.type === "redaction.secret");
  const readsSecret = (consumes ?? []).some((p) => p.type === "redaction.secret");
  if (writesSecret && readsSecret) {
    merged.set(VAULT_KEY, {
      ...VAULT_WRITE,
      direction: "both",
      description: t("Vault: reads and writes redaction secrets"),
    });
  } else if (writesSecret) {
    merged.set(VAULT_KEY, VAULT_WRITE);
  } else if (readsSecret) {
    merged.set(VAULT_KEY, VAULT_READ);
  }
  return [...merged.values()];
}
