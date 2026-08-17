import test from "node:test";
import assert from "node:assert/strict";
import { listDemoIds, loadManifest } from "./manifest.ts";
import { localizeManifest } from "./locale.ts";
import type { DemoManifest } from "../types.ts";

/**
 * The sidecars are loop output, and the loop wrote structure into them (#2032):
 * `id: oppdag` where the master says `discover`, `kind: skrivebord` where it
 * says `terminal`. A scene id is what an overlay is MATCHED BY, so a translated
 * id silently drops that scene, and localizeManifest then refuses a published
 * demo with incomplete coverage — 13 Norwegian renders could not be built at
 * all, the three sample walkthroughs among them.
 *
 * Every gate between the loop and these files asked which files it touched.
 * This one asks what is in them, by doing the thing the renderer does.
 */

/** Locales a committed sidecar exists for, across the whole demo tree. */
function committedLocales(manifests: DemoManifest[]): string[] {
  const locales = new Set<string>();
  for (const m of manifests) {
    for (const l of Object.keys(m.locales ?? {})) locales.add(l);
  }
  return [...locales].sort();
}

const manifests = listDemoIds().map((id) => loadManifest(id));

test("every committed narration overlay localizes the demo it belongs to", () => {
  assert.ok(manifests.length > 0, "the demo tree must hold manifests to check");
  for (const locale of committedLocales(manifests)) {
    for (const m of manifests) {
      if (!m.locales?.[locale]) continue;
      assert.doesNotThrow(
        () => localizeManifest(m, locale),
        `demo ${m.id}: its ${locale} sidecar cannot be applied — regenerate it with 'make l10n'`,
      );
    }
  }
});

test("a narration overlay never renames a scene or its kind", () => {
  for (const m of manifests) {
    for (const [locale, overlay] of Object.entries(m.locales ?? {})) {
      const masterIds = new Set(m.narration.map((s) => s.id));
      for (const o of overlay.narration) {
        assert.ok(
          masterIds.has(o.id),
          `demo ${m.id}: the ${locale} sidecar carries scene "${o.id}", which the master does not have — ` +
            `a scene id is structure the recipe never declares translatable`,
        );
      }
    }
  }
});

test("a published demo has full coverage in every locale it ships a sidecar for", () => {
  const published = manifests.filter((m) => m.publishAs);
  assert.ok(published.length > 0, "the gate is about published demos, so there must be some");
  for (const m of published) {
    for (const locale of Object.keys(m.locales ?? {})) {
      const localized = localizeManifest(m, locale);
      assert.equal(
        localized.narration.length,
        m.narration.length,
        `demo ${m.id}: the ${locale} render must carry every scene the master has`,
      );
    }
  }
});
