/**
 * Runs each example Block through the reference renderer and
 * compares against the hand-computed expected HTML. Treats a
 * mismatch as a design flaw (either the schema is ambiguous or
 * the renderer is wrong) and exits non-zero so we catch it
 * during development.
 *
 * Run with: node --experimental-strip-types examples/validate.ts
 */

// Minimal inline declaration so this draft package stays
// dependency-free. Replace with @types/node once the package
// moves into a repo that already has it.
declare const process: { exit(code: number): never };

import { renderBlockHtml, validateTargetAgainstSource } from '../src/preview.ts';
import type { Run, Block } from '../src/block.ts';
import {
  validateAnchor,
  type Annotation,
  type AnnotationAnchor,
} from '../src/annotation.ts';
import { filesHeading, filesHeadingExpectedHtml } from './files-heading.ts';
import { tagChip, tagChipExpectedHtml } from './tag-chip.ts';
import { shoppingCart, shoppingCartExpectedHtml } from './shopping-cart.ts';
import { exampleAnnotations } from './annotations.ts';

interface Case {
  name: string;
  block: Parameters<typeof renderBlockHtml>[0];
  expected: string;
}

const cases: Case[] = [
  { name: 'files-heading', block: filesHeading, expected: filesHeadingExpectedHtml },
  { name: 'tag-chip', block: tagChip, expected: tagChipExpectedHtml },
  { name: 'shopping-cart', block: shoppingCart, expected: shoppingCartExpectedHtml },
];

let failed = 0;

for (const c of cases) {
  const actual = renderBlockHtml(c.block);
  const ok = actual === c.expected;
  if (ok) {
    console.log(`\u2713 ${c.name}`);
  } else {
    failed++;
    console.log(`\u2717 ${c.name}`);
    console.log('  expected:', c.expected);
    console.log('  actual:  ', actual);
  }
}

if (failed > 0) {
  console.error(`\n${failed}/${cases.length} case(s) failed`);
  process.exit(1);
}
console.log(`\n${cases.length}/${cases.length} render cases passed`);

// ─── Merge-time validation scenarios ──────────────────────────────

// Scenario A: target preserves the {count} placeholder → valid.
const validTarget: Run[] = [
  { text: 'Dateien ' },
  {
    ph: {
      id: '1',
      type: 'jsx:var',
      subType: 'number',
      data: '{count}',
      equiv: 'count',
    },
  },
];
const errorsA = validateTargetAgainstSource(
  { ...filesHeading, placeholders: filesHeading.placeholders.filter(p => p.name === 'count') },
  validTarget,
);
console.log(`validator/valid: ${errorsA.length === 0 ? '\u2713' : '\u2717'} (${errorsA.length} errors)`);

// Scenario B: target drops the {count} placeholder → invalid.
const invalidTarget: Run[] = [{ text: 'Dateien' }];
const errorsB = validateTargetAgainstSource(
  { ...filesHeading, placeholders: filesHeading.placeholders.filter(p => p.name === 'count') },
  invalidTarget,
);
console.log(`validator/missing: ${errorsB.length === 1 ? '\u2713' : '\u2717'} (${errorsB.length} errors)`);
if (errorsB.length === 1) {
  console.log(`  message: ${errorsB[0].message}`);
}

// Scenario C: optional placeholder dropped → valid.
// TagChip's "badge" and "required" placeholders are optional, so
// a target that omits them must not produce an error (only
// "label" is required).
const tagChipWithoutOptionals: Run[] = [
  { text: 'Label ' },
  {
    ph: {
      id: '1',
      type: 'jsx:var',
      subType: 'string',
      data: '{label}',
      equiv: 'label',
    },
  },
];
const errorsC = validateTargetAgainstSource(tagChip, tagChipWithoutOptionals);
console.log(`validator/optional-drop: ${errorsC.length === 0 ? '\u2713' : '\u2717'} (${errorsC.length} errors)`);

// Scenario D: plural target preserves the pivot (via a nested
// plural run that references it). Validates that the pivot is
// correctly collected from inside a structured plural construct.
const germanPluralTarget: Run[] = [
  {
    plural: {
      pivot: 'count',
      forms: {
        one: [{ text: '1 Artikel im Warenkorb' }],
        other: [
          {
            ph: {
              id: '1',
              type: 'jsx:var',
              subType: 'number',
              data: '{count}',
              equiv: 'count',
            },
          },
          { text: ' Artikel im Warenkorb' },
        ],
      },
    },
  },
];
const errorsD = validateTargetAgainstSource(shoppingCart, germanPluralTarget);
console.log(`validator/plural-preserves-pivot: ${errorsD.length === 0 ? '\u2713' : '\u2717'} (${errorsD.length} errors)`);

// ─── Annotation anchor resolution ─────────────────────────────────

const blockById: Record<string, Block> = {
  'files-heading': filesHeading,
  'tag-chip': tagChip,
  'shopping-cart-plural': shoppingCart,
};

function checkAnnotation(
  label: string,
  annotation: Annotation,
  expectOk: boolean,
  blockOverride?: Block,
): void {
  // Always run against some block. If the anchor's block id isn't
  // in the fixture set and no override is provided, we pass one
  // arbitrary block in so that the resolver returns block-not-found
  // (simulating "the block was deleted by a re-extraction").
  const block =
    blockOverride ?? blockById[annotation.anchor.block] ?? filesHeading;
  const err = validateAnchor(block, annotation);
  const ok = err === null;
  const pass = ok === expectOk;
  const sym = pass ? '\u2713' : '\u2717';
  if (err && expectOk) {
    console.log(`annotation/${label}: ${sym} unexpectedly failed: ${err.message}`);
  } else if (!err && !expectOk) {
    console.log(`annotation/${label}: ${sym} unexpectedly resolved`);
  } else {
    console.log(`annotation/${label}: ${sym}`);
  }
}

// Every annotation in the example file should resolve cleanly.
console.log('');
for (const annotation of exampleAnnotations.annotations) {
  checkAnnotation(annotation.id, annotation, true);
}

// Synthetic orphan scenarios: each should fail to resolve.
const orphanMissingBlock: Annotation = {
  type: 'annotation',
  id: 'orphan-missing-block',
  anchor: { kind: 'block', block: 'does-not-exist' },
  data: {},
};
checkAnnotation('orphan-missing-block', orphanMissingBlock, false);

const orphanBadRunId: Annotation = {
  type: 'annotation',
  id: 'orphan-run-id-mismatch',
  anchor: {
    kind: 'run',
    block: 'tag-chip',
    path: [2],
    runId: '99', // actual run id at path [2] is "2"
  },
  data: {},
};
checkAnnotation('orphan-run-id-mismatch', orphanBadRunId, false);

const orphanMissingForm: Annotation = {
  type: 'annotation',
  id: 'orphan-missing-form',
  anchor: {
    kind: 'form',
    block: 'shopping-cart-plural',
    path: [0],
    key: 'few', // the shopping-cart plural has zero/one/other, no 'few'
  },
  data: {},
};
checkAnnotation('orphan-missing-form', orphanMissingForm, false);

const orphanBadRange: Annotation = {
  type: 'annotation',
  id: 'orphan-range-out-of-bounds',
  anchor: {
    kind: 'range',
    block: 'files-heading',
    path: [0],
    offset: 0,
    length: 9999, // way past the end of "Files "
  },
  data: {},
};
checkAnnotation('orphan-range-out-of-bounds', orphanBadRange, false);
