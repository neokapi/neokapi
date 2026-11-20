export type Profile = Record<string, Record<string, string>>;
export type History = {
  date: string;
  revision?: string;
  formats: Partial<Record<string, string>>;
  outputs?: Profile;
  connectors?: Profile;
};

export function operationChanges(current?: Profile, previous?: Profile) {
  if (!current || !previous) return [];
  const changes = [];
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
