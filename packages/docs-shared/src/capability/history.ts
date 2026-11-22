/** Each subject's operations and their evidence states in one observation. */
export type Profile = Record<string, Record<string, string>>;

export type OperationChange = {
  subject: string;
  operation: string;
  before?: string;
  after?: string;
};

/** List the operations whose state differs between two observations. */
export function operationChanges(current?: Profile, previous?: Profile): OperationChange[] {
  if (!current || !previous) return [];
  const changes: OperationChange[] = [];
  for (const subject of [...new Set([...Object.keys(current), ...Object.keys(previous)])].sort()) {
    const now = current[subject] ?? {};
    const before = previous[subject] ?? {};
    for (const operation of [...new Set([...Object.keys(now), ...Object.keys(before)])].sort()) {
      if (before[operation] !== now[operation]) {
        changes.push({ subject, operation, before: before[operation], after: now[operation] });
      }
    }
  }
  return changes;
}
