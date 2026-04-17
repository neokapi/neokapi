import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react-vite';

import type { Block, Run } from '@neokapi/kapi-format';
import { PluralTargetEditor } from '../../components/plural/PluralTargetEditor';

const meta: Meta<typeof PluralTargetEditor> = {
  title: 'Editor/Plural/PluralTargetEditor',
  component: PluralTargetEditor,
  tags: ['autodocs'],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          'Translator-facing editor for a single locale\'s target. When the source is a flat sentence but the target locale needs plural handling, the translator can upgrade the target into a `PluralRun` without touching the source. Switches back and forth between flat textarea and per-form textareas via the same component.',
      },
    },
  },
};

export default meta;

type Story = StoryObj<typeof PluralTargetEditor>;

const block: Pick<Block, 'source' | 'placeholders'> = {
  source: [
    { text: 'You have ' },
    { ph: { id: '1', type: 'jsx:var', data: '{count}', equiv: 'count' } },
    { text: ' messages' },
  ],
  placeholders: [
    { name: 'count', kind: 'variable', jsType: 'number', sourceExpr: 'count' },
  ],
};

const flatGerman: Run[] = [
  { text: 'Sie haben ' },
  { ph: { id: '1', type: 'jsx:var', data: '{count}', equiv: 'count' } },
  { text: ' Nachrichten' },
];

const pluralGerman: Run[] = [
  {
    plural: {
      pivot: 'count',
      forms: {
        zero: [{ text: 'Keine Nachrichten' }],
        one: [{ text: '1 Nachricht' }],
        other: flatGerman,
      },
    },
  },
];

function Interactive(initial: Run[]): Story['render'] {
  return () => {
    const [target, setTarget] = useState<Run[]>(initial);
    return <PluralTargetEditor block={block} target={target} onChange={setTarget} />;
  };
}

export const FlatTarget: Story = {
  name: 'Flat target (upgrade available)',
  render: Interactive(flatGerman),
};

export const EmptyFlat: Story = {
  name: 'Empty target (new locale)',
  render: Interactive([]),
};

export const FullPlural: Story = {
  name: 'Plural target (downgrade available)',
  render: Interactive(pluralGerman),
};

export const PluralPartiallyFilled: Story = {
  name: 'Plural target partially filled',
  render: Interactive([
    {
      plural: {
        pivot: 'count',
        forms: {
          zero: [],
          one: [{ text: '1 Nachricht' }],
          other: [],
        },
      },
    },
  ]),
};

export const MultiplePlaceholders: Story = {
  name: 'Block with multiple placeholder candidates',
  render: () => {
    const richBlock: Pick<Block, 'source' | 'placeholders'> = {
      source: [
        { text: 'User ' },
        { ph: { id: '1', type: 'jsx:var', data: '{name}', equiv: 'name' } },
        { text: ' opened ' },
        { ph: { id: '2', type: 'jsx:var', data: '{count}', equiv: 'count' } },
        { text: ' files in ' },
        { ph: { id: '3', type: 'jsx:var', data: '{folder}', equiv: 'folder' } },
      ],
      placeholders: [
        { name: 'name', kind: 'variable', jsType: 'string', sourceExpr: 'user.name' },
        { name: 'count', kind: 'variable', jsType: 'number', sourceExpr: 'file.count' },
        { name: 'folder', kind: 'variable', jsType: 'string', sourceExpr: 'folder.name' },
      ],
    };
    const [target, setTarget] = useState<Run[]>([
      { text: 'Benutzer ' },
      { ph: { id: '1', type: 'jsx:var', data: '{name}', equiv: 'name' } },
      { text: ' hat ' },
      { ph: { id: '2', type: 'jsx:var', data: '{count}', equiv: 'count' } },
      { text: ' Dateien geöffnet in ' },
      { ph: { id: '3', type: 'jsx:var', data: '{folder}', equiv: 'folder' } },
    ]);
    return <PluralTargetEditor block={richBlock} target={target} onChange={setTarget} />;
  },
};
