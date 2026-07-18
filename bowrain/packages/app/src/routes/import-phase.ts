import type { ConnectorSyncStatus } from "@neokapi/ui";

/** Where a freshly bound repository's initial import stands. */
export type ImportPhase = "pending" | "ready" | "failed";

/**
 * Reads the initial-import phase off the connector's polled sync status — the
 * same surface the connectors panel renders. A recorded sync error means the
 * background ingest failed; a real first-sync timestamp means the repository's
 * content landed; anything else is still in flight. `lastSync` is
 * authoritative: the server stamps it only after a successful fetch, and a
 * never-synced connector serializes it as a year-1 (or empty) timestamp the
 * panel already treats as "never synced".
 */
export function importPhaseFromStatus(
  status: Pick<ConnectorSyncStatus, "lastSync" | "errors"> | undefined,
): ImportPhase {
  if (!status) return "pending";
  if (status.errors.length > 0) return "failed";
  const lastSync = status.lastSync ? new Date(status.lastSync) : null;
  if (lastSync && !Number.isNaN(lastSync.getTime()) && lastSync.getTime() > 0) return "ready";
  return "pending";
}
