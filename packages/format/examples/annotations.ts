/**
 * Example annotation file targeting the three example blocks.
 *
 * Three annotation records cover the four anchor shapes:
 *
 *   - BlockAnchor: review status on FilesHeading.
 *   - RunAnchor: protected term on TagChip's `label` ph run.
 *   - FormAnchor: MT confidence on the 'other' form of the
 *     ShoppingCart plural run.
 *   - RangeAnchor: a glossary match inside a text run.
 *
 * The file shape mirrors what an actual
 * `annotations/example.klfl` file would look like after
 * JSON-Lines parsing: one header record, N annotation records.
 */

import type { AnnotationFile } from '../src/annotation.ts';

export const exampleAnnotations: AnnotationFile = {
  header: {
    type: 'header',
    annotationType: '@neokapi/example',
    annotationVersion: '1.0.0',
    producer: {
      id: '@neokapi/format-examples',
      version: '0.0.1',
    },
    created: '2026-04-15T12:00:00Z',
    targetArchive: 'sha256:deadbeef',
  },

  annotations: [
    // Block-level: FilesHeading has been reviewed and approved for de.
    {
      type: 'annotation',
      id: 'review-1',
      anchor: {
        kind: 'block',
        block: 'files-heading',
      },
      data: {
        kind: 'review',
        locale: 'de',
        status: 'approved',
        reviewer: 'alice@example.com',
        approvedAt: '2026-04-15T11:45:00Z',
      },
    },

    // Run-level: TagChip's `label` placeholder (ph run id "2") is
    // tagged as a protected term that should not be translated.
    // The path [1] steps to the 2nd run of block.source — which is
    // the text run " " (a space between the badge and label
    // chips). Wait, that's wrong. Let me recount:
    //   index 0: ph "1" (badge)
    //   index 1: text " "
    //   index 2: ph "2" (label)  ← target
    //   index 3: text " "
    //   index 4: ph "3" (required)
    // So the correct path is [2].
    {
      type: 'annotation',
      id: 'term-1',
      anchor: {
        kind: 'run',
        block: 'tag-chip',
        path: [2],
        runId: '2',
      },
      data: {
        kind: 'protected-term',
        term: 'label',
        termbaseEntry: 'ui-terminology:label',
        action: 'preserve-placeholder',
        confidence: 1.0,
      },
    },

    // Form-level: the 'other' plural form of ShoppingCart was
    // machine-translated with a confidence score. Path [0] points
    // at the single top-level run of block.source (the plural
    // run itself); the key "other" selects the 'other' form.
    {
      type: 'annotation',
      id: 'mt-1',
      anchor: {
        kind: 'form',
        block: 'shopping-cart-plural',
        path: [0],
        key: 'other',
      },
      data: {
        kind: 'mt-confidence',
        locale: 'de',
        engine: 'deepl',
        model: 'v2',
        confidence: 0.87,
      },
    },

    // Range-level: characters 6-12 of the first text run of the
    // FilesHeading block ("Files ") — 5 characters is "Files".
    // In practice this would flag a glossary match on "Files".
    {
      type: 'annotation',
      id: 'term-2',
      anchor: {
        kind: 'range',
        block: 'files-heading',
        path: [0],
        offset: 0,
        length: 5,
      },
      data: {
        kind: 'glossary-match',
        term: 'Files',
        termbaseEntry: 'ui-terminology:files',
        targetLocaleSuggestions: {
          de: 'Dateien',
          ja: 'ファイル',
        },
      },
    },
  ],
};
