/**
 * Example 2 — TagChip
 *
 * Exercises: conditional JSX expressions captured as jsx:node
 * placeholders, optional flag on placeholders, {label} as a
 * variable alongside two ReactNode-valued expressions.
 *
 * Source:
 *   export function TagChip({ index, label, deletable }) {
 *     return (
 *       <span data-tag-chip>
 *         {index !== undefined && <span className="badge">{index}</span>}
 *         {label}
 *         {!deletable && <span className="required">*</span>}
 *       </span>
 *     );
 *   }
 *
 * Translator view (Level-1 preview): three chips separated by
 * spaces. The two conditional JSX expressions are `jsx:node`
 * placeholders with `optional: true` because a language may
 * legitimately drop them.
 */

import type { Block } from '../src/block.ts';

export const tagChip: Block = {
  id: 'tag-chip',
  hash: '2GcSuQ',
  translatable: true,
  type: 'jsx:element',

  source: [
    {
      ph: {
        id: '1',
        type: 'jsx:node',
        subType: 'logical-and',
        data: 'index !== undefined && <span className="badge">{index}</span>',
        equiv: 'badge',
        disp: '⟨badge⟩',
      },
    },
    { text: ' ' },
    {
      ph: {
        id: '2',
        type: 'jsx:var',
        subType: 'string',
        data: '{label}',
        equiv: 'label',
        disp: 'label',
      },
    },
    { text: ' ' },
    {
      ph: {
        id: '3',
        type: 'jsx:node',
        subType: 'logical-and',
        data: '!deletable && <span className="required">*</span>',
        equiv: 'required',
        disp: '⟨required⟩',
      },
    },
  ],

  placeholders: [
    {
      name: 'badge',
      kind: 'node',
      sourceExpr: 'index !== undefined && <span className="badge">{index}</span>',
      jsType: 'ReactNode',
      optional: true,
    },
    {
      name: 'label',
      kind: 'variable',
      sourceExpr: 'label',
      jsType: 'string',
    },
    {
      name: 'required',
      kind: 'node',
      sourceExpr: '!deletable && <span className="required">*</span>',
      jsType: 'ReactNode',
      optional: true,
    },
  ],

  properties: {
    file: 'src/TagChip.tsx',
    line: 3,
    component: 'TagChip',
    jsxPath: 'TagChip > span[data-tag-chip]',
    element: 'span',
    locNote: 'Tag chip shown in the sidebar list of filters.',
  },

  preview: {
    storyId: 'components-tagchip--default',
    sampleValues: { label: 'react', index: 3, deletable: true },
  },
};

export const tagChipExpectedHtml =
  '<kat-block id="tag-chip" data-type="jsx:element">' +
  '<span class="neokapi-node" data-node="1" title="index !== undefined &amp;&amp; &lt;span className=&quot;badge&quot;&gt;{index}&lt;/span&gt;">badge</span>' +
  ' ' +
  '<span class="neokapi-var" data-var="label" data-type="string">label</span>' +
  ' ' +
  '<span class="neokapi-node" data-node="3" title="!deletable &amp;&amp; &lt;span className=&quot;required&quot;&gt;*&lt;/span&gt;">required</span>' +
  '</kat-block>';
