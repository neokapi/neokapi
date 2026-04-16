/**
 * Example 3 — ShoppingCart with <Plural>
 *
 * Exercises: a structured plural run containing typed inner runs
 * (including a variable placeholder in the 'other' form), the
 * `icu-pivot` placeholder kind, and the flat Block shape around a
 * structured plural construct.
 *
 * Source:
 *   export function ShoppingCart({ items }: { items: number }) {
 *     return (
 *       <p>
 *         <Plural
 *           count={items}
 *           zero="Your cart is empty"
 *           one="1 item in your cart"
 *           other="{count} items in your cart"
 *         />
 *       </p>
 *     );
 *   }
 *
 * The extractor emits ONE Block with `source: [{ plural: … }]`.
 * The plural's `forms` map carries a Run[] per plural form, each
 * with its own typed content (text runs + a ph run for {count} in
 * the 'other' form). The pivot variable is declared in the Block's
 * placeholders list with kind 'icu-pivot'.
 *
 * This is the shape that lets inline markup inside plural clauses
 * stay first-class. A plural form whose text contained, say,
 * `<strong>{count}</strong>` would have a pcOpen / ph / pcClose
 * triple inside its forms entry — same run-level primitives as any
 * other text.
 */

import type { Block } from '../src/block.ts';

export const shoppingCart: Block = {
  id: 'shopping-cart-plural',
  hash: '9QpZ11',
  translatable: true,
  type: 'jsx:element',

  source: [
    {
      plural: {
        pivot: 'count',
        forms: {
          zero: [{ text: 'Your cart is empty' }],
          one: [{ text: '1 item in your cart' }],
          other: [
            {
              ph: {
                id: '1',
                type: 'jsx:var',
                subType: 'number',
                data: '{count}',
                equiv: 'count',
                disp: 'count',
              },
            },
            { text: ' items in your cart' },
          ],
        },
      },
    },
  ],

  placeholders: [
    {
      // Pivot variable driving the plural selection. Declared here
      // so validators know it must be preserved in every target's
      // plural run (and so every target's plural or ICU expansion
      // references the same variable).
      name: 'count',
      kind: 'icu-pivot',
      sourceExpr: 'items',
      jsType: 'number',
    },
  ],

  properties: {
    file: 'src/ShoppingCart.tsx',
    line: 4,
    component: 'ShoppingCart',
    jsxPath: 'ShoppingCart > p > Plural',
    element: 'Plural',
  },

  preview: {
    sampleValues: { count: 3 },
  },
};

export const shoppingCartExpectedHtml =
  '<kat-block id="shopping-cart-plural" data-type="jsx:element">' +
  '<span class="neokapi-plural" data-pivot="count">' +
  '<div class="neokapi-plural-form" data-form="zero">' +
  '<span class="neokapi-plural-form-label">plural:zero</span>' +
  'Your cart is empty' +
  '</div>' +
  '<div class="neokapi-plural-form" data-form="one">' +
  '<span class="neokapi-plural-form-label">plural:one</span>' +
  '1 item in your cart' +
  '</div>' +
  '<div class="neokapi-plural-form" data-form="other">' +
  '<span class="neokapi-plural-form-label">plural:other</span>' +
  '<span class="neokapi-var" data-var="count" data-type="number">count</span>' +
  ' items in your cart' +
  '</div>' +
  '</span>' +
  '</kat-block>';
