// @vitest-environment jsdom
import { act, createElement } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { Block, Run } from '@neokapi/kapi-format';
import { PluralTargetEditor } from '../components/plural/PluralTargetEditor';

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement('div');
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

afterEach(() => {
  // jsdom cleanup — remove every mounted container between tests.
  for (const node of Array.from(document.body.childNodes)) {
    document.body.removeChild(node);
  }
});

function fixtureBlock(): Pick<Block, 'source' | 'placeholders'> {
  return {
    source: [
      { text: 'You have ' },
      { ph: { id: '1', type: 'jsx:var', data: '{count}', equiv: 'count' } },
      { text: ' messages' },
    ],
    placeholders: [
      { name: 'count', kind: 'variable', jsType: 'number', sourceExpr: 'count' },
    ],
  };
}

function flatGermanTarget(): Run[] {
  return [
    { text: 'Sie haben ' },
    { ph: { id: '1', type: 'jsx:var', data: '{count}', equiv: 'count' } },
    { text: ' Nachrichten' },
  ];
}

function type(textarea: HTMLTextAreaElement, value: string): void {
  // React intercepts `value` assignments via a prototype descriptor
  // so change events fire for controlled components. Writing through
  // the native setter preserves that contract.
  const setter = Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    'value',
  )?.set;
  act(() => {
    setter?.call(textarea, value);
    textarea.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

function click(button: HTMLButtonElement): void {
  act(() => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  });
}

describe('<PluralTargetEditor>', () => {
  it('renders a flat textarea with the current target text', () => {
    const onChange = vi.fn<(next: Run[]) => void>();
    const c = renderToContainer(
      createElement(PluralTargetEditor, {
        block: fixtureBlock(),
        target: flatGermanTarget(),
        onChange,
      }),
    );
    const textarea = c.querySelector('textarea');
    expect(textarea).toBeTruthy();
    expect((textarea as HTMLTextAreaElement).value).toBe('Sie haben {count} Nachrichten');
    expect(c.querySelector('[data-neokapi-plural-editor="flat"]')).toBeTruthy();
  });

  it('emits a typed Run[] when the translator edits the flat target', () => {
    const onChange = vi.fn<(next: Run[]) => void>();
    const c = renderToContainer(
      createElement(PluralTargetEditor, {
        block: fixtureBlock(),
        target: flatGermanTarget(),
        onChange,
      }),
    );
    const textarea = c.querySelector('textarea') as HTMLTextAreaElement;
    type(textarea, 'Hallo, {count} Nachrichten');

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.calls[0][0];
    expect(next).toEqual([
      { text: 'Hallo, ' },
      expect.objectContaining({ ph: expect.objectContaining({ equiv: 'count' }) }),
      { text: ' Nachrichten' },
    ]);
  });

  it('upgrades flat → plural using the chosen pivot', () => {
    const onChange = vi.fn<(next: Run[]) => void>();
    const c = renderToContainer(
      createElement(PluralTargetEditor, {
        block: fixtureBlock(),
        target: flatGermanTarget(),
        onChange,
      }),
    );
    const buttons = Array.from(c.querySelectorAll('button'));
    const upgrade = buttons.find((b) => b.textContent?.includes('Upgrade'));
    expect(upgrade).toBeTruthy();
    click(upgrade as HTMLButtonElement);

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.calls[0][0];
    expect(next).toHaveLength(1);
    expect(next[0]).toHaveProperty('plural');
  });

  it('renders per-form textareas when the target is a PluralRun', () => {
    const pluralTarget: Run[] = [
      {
        plural: {
          pivot: 'count',
          forms: {
            one: [{ text: '1 Nachricht' }],
            other: flatGermanTarget(),
          },
        },
      },
    ];
    const c = renderToContainer(
      createElement(PluralTargetEditor, {
        block: fixtureBlock(),
        target: pluralTarget,
        onChange: () => {},
        forms: ['one', 'other'],
      }),
    );
    const textareas = Array.from(c.querySelectorAll('textarea')) as HTMLTextAreaElement[];
    expect(textareas).toHaveLength(2);
    expect(textareas[0].value).toBe('1 Nachricht');
    expect(textareas[1].value).toBe('Sie haben {count} Nachrichten');
    expect(c.querySelector('[data-neokapi-plural-editor="plural"]')).toBeTruthy();
  });

  it('emits a new Run[] when a single plural form is edited', () => {
    const onChange = vi.fn<(next: Run[]) => void>();
    const pluralTarget: Run[] = [
      {
        plural: {
          pivot: 'count',
          forms: {
            one: [{ text: '1 Nachricht' }],
            other: flatGermanTarget(),
          },
        },
      },
    ];
    const c = renderToContainer(
      createElement(PluralTargetEditor, {
        block: fixtureBlock(),
        target: pluralTarget,
        onChange,
        forms: ['one', 'other'],
      }),
    );
    const textareas = Array.from(c.querySelectorAll('textarea')) as HTMLTextAreaElement[];
    type(textareas[0], 'Genau 1 Nachricht');

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.calls[0][0];
    const wrapper = next[0] as Extract<Run, { plural: unknown }>;
    expect(wrapper.plural.forms.one).toEqual([{ text: 'Genau 1 Nachricht' }]);
    expect(wrapper.plural.forms.other).toEqual(flatGermanTarget());
  });

  it('downgrades plural → flat (keeping the `other` form) on demand', () => {
    const onChange = vi.fn<(next: Run[]) => void>();
    const pluralTarget: Run[] = [
      {
        plural: {
          pivot: 'count',
          forms: {
            one: [{ text: '1 Nachricht' }],
            other: flatGermanTarget(),
          },
        },
      },
    ];
    const c = renderToContainer(
      createElement(PluralTargetEditor, {
        block: fixtureBlock(),
        target: pluralTarget,
        onChange,
      }),
    );
    const downgrade = Array.from(c.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Flatten'),
    );
    expect(downgrade).toBeTruthy();
    click(downgrade as HTMLButtonElement);

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0][0]).toEqual(flatGermanTarget());
  });
});
