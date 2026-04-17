import { describe, expect, it } from 'vitest';

import type { Block, Run } from '../src/index.ts';
import {
  downgradePluralTarget,
  isPlural,
  pluralPivotCandidates,
  pluralTargetPivot,
  setPluralForm,
  upgradeTargetToPlural,
} from '../src/target-plural.ts';

function flatGermanTarget(): Run[] {
  return [
    { text: 'Sie haben ' },
    { ph: { id: '1', type: 'jsx:var', data: '{count}', equiv: 'count' } },
    { text: ' Nachrichten' },
  ];
}

describe('upgradeTargetToPlural', () => {
  it('wraps the existing flat target in a PluralRun with it as the `other` form', () => {
    const flat = flatGermanTarget();
    const upgraded = upgradeTargetToPlural(flat, 'count');
    expect(upgraded).toHaveLength(1);
    expect(isPlural(upgraded)).toBe(true);
    expect(pluralTargetPivot(upgraded)).toBe('count');
    const wrapper = upgraded[0] as Extract<Run, { plural: unknown }>;
    expect(wrapper.plural.forms.other).toEqual(flat);
  });

  it('initialises the remaining plural forms empty', () => {
    const upgraded = upgradeTargetToPlural(flatGermanTarget(), 'count');
    const wrapper = upgraded[0] as Extract<Run, { plural: unknown }>;
    expect(wrapper.plural.forms.zero).toEqual([]);
    expect(wrapper.plural.forms.one).toEqual([]);
  });

  it('is idempotent when the target is already plural', () => {
    const once = upgradeTargetToPlural(flatGermanTarget(), 'count');
    const twice = upgradeTargetToPlural(once, 'count');
    expect(twice).toEqual(once);
  });

  it('accepts a custom form list', () => {
    const upgraded = upgradeTargetToPlural(flatGermanTarget(), 'count', ['one', 'other']);
    const wrapper = upgraded[0] as Extract<Run, { plural: unknown }>;
    expect(Object.keys(wrapper.plural.forms).sort()).toEqual(['one', 'other']);
  });
});

describe('downgradePluralTarget', () => {
  it('collapses back to the `other` form', () => {
    const upgraded = upgradeTargetToPlural(flatGermanTarget(), 'count');
    const back = downgradePluralTarget(upgraded);
    expect(back).toEqual(flatGermanTarget());
  });

  it('returns flat runs unchanged when the target is already flat', () => {
    const flat = flatGermanTarget();
    expect(downgradePluralTarget(flat)).toEqual(flat);
  });

  it('returns empty when the target is empty or undefined', () => {
    expect(downgradePluralTarget(undefined)).toEqual([]);
    expect(downgradePluralTarget([])).toEqual([]);
  });
});

describe('setPluralForm', () => {
  it('replaces a single form without touching the others', () => {
    const upgraded = upgradeTargetToPlural(flatGermanTarget(), 'count');
    const next = setPluralForm(upgraded, 'one', [{ text: '1 Nachricht' }]);
    const wrapper = next[0] as Extract<Run, { plural: unknown }>;
    expect(wrapper.plural.forms.one).toEqual([{ text: '1 Nachricht' }]);
    expect(wrapper.plural.forms.other).toEqual(flatGermanTarget());
  });

  it('no-ops on a flat target', () => {
    const flat = flatGermanTarget();
    expect(setPluralForm(flat, 'one', [{ text: '1 item' }])).toEqual(flat);
  });
});

describe('pluralPivotCandidates', () => {
  const block: Block = {
    id: 'b',
    hash: 'h',
    translatable: true,
    type: 'jsx:element',
    source: [{ text: 'Hello ' }],
    placeholders: [
      { name: 'user.name', kind: 'variable', sourceExpr: 'user.name' },
      { name: 'count', kind: 'variable', jsType: 'number', sourceExpr: 'count' },
      { name: 'role', kind: 'icu-pivot', jsType: 'string', sourceExpr: 'role' },
    ],
    properties: {
      file: 'x.tsx',
      line: 1,
      component: 'X',
      jsxPath: 'p',
      element: 'p',
    },
  };

  it('surfaces icu-pivot placeholders first, then numeric, then others', () => {
    const cands = pluralPivotCandidates(block);
    expect(cands.map((c) => c.name)).toEqual(['role', 'count', 'user.name']);
    expect(cands[0].sourcePivot).toBe(true);
    expect(cands[1].sourcePivot).toBe(false);
  });
});
