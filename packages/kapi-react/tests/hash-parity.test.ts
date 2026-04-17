/**
 * Hash parity regression test.
 *
 * `Block.hash` in the .klz extract output must equal the `hash`
 * argument the plugin transform stamps into every `__t()` / `__tx()`
 * call at build time. If they drift, the kapi-react runtime dict
 * loaded via `loadTranslations()` stops resolving. The test walks a
 * set of representative fixtures through both sides and asserts
 * every extract-side hash exists in the transform output.
 */

import { describe, expect, it } from 'vitest';

import { extractDocument } from '../src/extract/index.ts';
import { transform } from '../src/plugin/transform.ts';

const FIXTURES: ReadonlyArray<{ name: string; code: string }> = [
  {
    name: 'plain text',
    code: '<h1>Hello World</h1>',
  },
  {
    name: 'text with variable',
    code: '<h1>Hello, {name}!</h1>',
  },
  {
    name: 'text with member expression',
    code: '<p>Welcome, {user.name}!</p>',
  },
  {
    name: 'text with inline element',
    code: '<p>Click <a href="/x">here</a> to continue.</p>',
  },
  {
    name: 'attribute translation',
    code: '<input placeholder="Search..." />',
  },
  {
    name: 'multiple blocks in one file',
    code: `
      <div>
        <h1>Title</h1>
        <p>Body {count}.</p>
        <button aria-label="Close">Save</button>
      </div>
    `,
  },
  {
    name: 'inline child with own text',
    code: '<p>Press <kbd>Cmd</kbd>+<kbd>K</kbd> to search.</p>',
  },
  {
    name: 'nested blocks',
    code: '<section><h2>Title</h2><p>Body</p></section>',
  },
  {
    name: 'Plural with flat text forms',
    code: `<p><Plural count={n}>
      <One>1 item</One>
      <Other>{n} items</Other>
    </Plural></p>`,
  },
  {
    name: 'Plural with inline JSX inside a form',
    code: `<p><Plural count={items.length}>
      <Zero>Your cart is empty</Zero>
      <One>1 item</One>
      <Other><strong>{items.length}</strong> items in your cart</Other>
    </Plural></p>`,
  },
  {
    name: 'Select with literal cases',
    code: `<p><Select value={role}>
      <Case when="admin">Admin</Case>
      <Case when="guest">Guest</Case>
      <Other>User</Other>
    </Select></p>`,
  },
];

function hashesFromTransform(code: string): Set<string> {
  const out = transform(code, 'Test.tsx', { mode: 'runtime' });
  if (!out?.code) return new Set();
  // __t("hash", …)  and  __tx("hash", …)
  const hashes = new Set<string>();
  for (const match of out.code.matchAll(/__tx?\("([^"]+)"/g)) {
    hashes.add(match[1]);
  }
  return hashes;
}

function hashesFromExtract(code: string): Set<string> {
  const doc = extractDocument(code, { filename: 'Test.tsx' });
  return new Set(doc?.blocks.map((b) => b.hash) ?? []);
}

describe('hash parity between extract and transform', () => {
  for (const { name, code } of FIXTURES) {
    it(`emits the same hashes for "${name}"`, () => {
      const extracted = hashesFromExtract(code);
      const transformed = hashesFromTransform(code);

      // Every extracted hash must be somewhere in the transform output.
      // (Transform may emit additional hashes the extract skips — e.g.
      // attribute-only elements still trigger extract; symmetry is
      // desirable, so we also check the reverse below.)
      for (const hash of extracted) {
        expect(transformed, `extract hash "${hash}" missing from transform`).toContain(hash);
      }

      // Reverse direction: every hash the transform emits must be one
      // the extractor produced. Catches cases where transform still
      // translates something extract correctly skips (or vice versa).
      for (const hash of transformed) {
        expect(extracted, `transform hash "${hash}" missing from extract`).toContain(hash);
      }
    });
  }

  it('covers every fixture — regression guard', () => {
    // Sanity: we're not shipping an empty fixture set.
    expect(FIXTURES.length).toBeGreaterThan(5);
  });
});
