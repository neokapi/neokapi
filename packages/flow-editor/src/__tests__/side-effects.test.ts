import { describe, it, expect } from "vitest";
import { getSystemEffects } from "../sideEffects";
import type { IOPort } from "../types";

const secret: IOPort = { type: "redaction.secret", side: "source" };
const source: IOPort = { type: "source", side: "source" };

describe("getSystemEffects", () => {
  it("maps declared side effects to systems", () => {
    const systems = getSystemEffects(["tm-read", "api-call"]);
    expect(systems.map((s) => s.key).sort()).toEqual(["api", "tm"]);
  });

  it("collapses read + write of the same system to a 'both' direction", () => {
    const systems = getSystemEffects(["tm-read", "tm-write"]);
    expect(systems).toHaveLength(1);
    expect(systems[0]).toMatchObject({ key: "tm", direction: "both" });
  });

  it("attaches a write-direction Vault when a tool produces a redaction secret", () => {
    // redact writes originals into the vault for later restore.
    const systems = getSystemEffects(undefined, [source, secret]);
    const vault = systems.find((s) => s.key === "vault");
    expect(vault).toMatchObject({ label: "Vault", direction: "write" });
  });

  it("attaches a read-direction Vault when a tool consumes a redaction secret", () => {
    // unredact reads originals back out of the vault to restore them.
    const systems = getSystemEffects(undefined, undefined, [secret]);
    const vault = systems.find((s) => s.key === "vault");
    expect(vault).toMatchObject({ label: "Vault", direction: "read" });
  });

  it("collapses a Vault to 'both' when a tool produces and consumes the secret", () => {
    const systems = getSystemEffects(undefined, [secret], [secret]);
    const vaults = systems.filter((s) => s.key === "vault");
    expect(vaults).toHaveLength(1);
    expect(vaults[0]).toMatchObject({ direction: "both" });
  });

  it("attaches no Vault when no redaction secret is produced or consumed", () => {
    const systems = getSystemEffects(["api-call"], [source], [source]);
    expect(systems.some((s) => s.key === "vault")).toBe(false);
  });
});
