import type { ApiAdapter } from "./adapter";
import type { ConnectorSyncStatus } from "../types/api";

/** The batch status endpoint answers at most this many connectors per call. */
export const CONNECTOR_STATUS_BATCH_SIZE = 100;

function chunk(ids: string[], size: number): string[][] {
  const out: string[][] = [];
  for (let i = 0; i < ids.length; i += size) out.push(ids.slice(i, i + size));
  return out;
}

/**
 * Read a whole panel's connector states, splitting the selection across as
 * many batch calls as the endpoint's id cap requires. The result is keyed by
 * connector id; an id the server could not answer for is simply absent, which
 * is what the batch's `unknown` list reports.
 *
 * `probe` is the deep read — reserve it for explicit manual surfaces. Polling
 * surfaces take the cheap default.
 */
export async function fetchConnectorStatuses(
  api: ApiAdapter,
  workspaceSlug: string,
  connectorIds: string[],
  opts?: { probe?: boolean },
): Promise<Record<string, ConnectorSyncStatus>> {
  const batches = await Promise.all(
    chunk(connectorIds, CONNECTOR_STATUS_BATCH_SIZE).map((ids) =>
      api.getConnectorStatuses(workspaceSlug, ids, opts),
    ),
  );
  const statuses: Record<string, ConnectorSyncStatus> = {};
  for (const batch of batches) Object.assign(statuses, batch.statuses);
  return statuses;
}
