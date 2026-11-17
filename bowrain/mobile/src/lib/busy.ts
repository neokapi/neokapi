// Whether the person is in the middle of something an app reload would
// throw away: an open sheet, text being typed. A screen marks itself busy
// while it holds such state; the update controller waits for it to clear.
const holders = new Set<string>();

export function markBusy(key: string, busy: boolean): void {
  if (busy) holders.add(key);
  else holders.delete(key);
}

export function isBusy(): boolean {
  return holders.size > 0;
}
