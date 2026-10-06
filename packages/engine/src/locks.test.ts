import { describe, expect, it } from "vitest";
import { LOCK_EXCLUSIVE, LOCK_NONE, LOCK_RESERVED, LOCK_SHARED, LockTable } from "./locks.ts";

const OK = 0;
const BUSY = 5;

describe("LockTable", () => {
  it("lets readers share a database", () => {
    const t = new LockTable();
    t.open(1, "/a.db");
    t.open(2, "/a.db");
    expect(t.lock(1, LOCK_SHARED)).toBe(OK);
    expect(t.lock(2, LOCK_SHARED)).toBe(OK);
  });

  it("refuses a second writer and a write beside a reader", () => {
    const t = new LockTable();
    t.open(1, "/a.db");
    t.open(2, "/a.db");
    expect(t.lock(1, LOCK_SHARED)).toBe(OK);
    expect(t.lock(1, LOCK_RESERVED)).toBe(OK);
    expect(t.lock(2, LOCK_SHARED)).toBe(OK);
    expect(t.lock(2, LOCK_RESERVED)).toBe(BUSY);
    // The writer cannot commit while the reader holds SHARED, and the pending
    // write keeps the reader from starting again once it lets go.
    expect(t.lock(1, LOCK_EXCLUSIVE)).toBe(BUSY);
    t.unlock(2, LOCK_NONE);
    expect(t.lock(2, LOCK_SHARED)).toBe(BUSY);
    expect(t.lock(1, LOCK_EXCLUSIVE)).toBe(OK);
    expect(t.reservedBy(2)).toBe(true);
    t.unlock(1, LOCK_SHARED);
    expect(t.reservedBy(2)).toBe(false);
    expect(t.lock(2, LOCK_SHARED)).toBe(OK);
  });

  it("keeps databases apart", () => {
    const t = new LockTable();
    t.open(1, "/a.db");
    t.open(2, "/b.db");
    expect(t.lock(1, LOCK_EXCLUSIVE)).toBe(OK);
    expect(t.lock(2, LOCK_EXCLUSIVE)).toBe(OK);
  });

  it("releases everything a closed handle held", () => {
    const t = new LockTable();
    t.open(1, "/a.db");
    t.open(2, "/a.db");
    expect(t.lock(1, LOCK_EXCLUSIVE)).toBe(OK);
    t.close(1);
    expect(t.lock(2, LOCK_EXCLUSIVE)).toBe(OK);
  });

  it("grants an untracked handle everything", () => {
    const t = new LockTable();
    expect(t.lock(9, LOCK_EXCLUSIVE)).toBe(OK);
  });
});
