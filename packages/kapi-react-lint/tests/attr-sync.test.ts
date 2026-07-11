/**
 * Drift guard: this package duplicates kapi-react's translatable
 * attribute vocabulary (to stay installable without the transform).
 * This test pins the copy to the canonical set — if kapi-react's
 * defaults change, this fails until the copy is updated.
 */

import { describe, expect, it } from "vitest";

import { translatableAttributes } from "../../kapi-react/src/plugin/defaults.ts";
import { TRANSLATABLE_ATTRS } from "../src/shared/translatable-attrs.ts";

describe("TRANSLATABLE_ATTRS ↔ kapi-react defaults", () => {
  it("matches the canonical vocabulary exactly", () => {
    expect([...TRANSLATABLE_ATTRS].sort()).toEqual([...translatableAttributes].sort());
  });
});
