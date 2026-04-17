/**
 * kapi-react extract tests — the AST walker emits structured
 * Block / Run sequences per AD-045. The flat-string extraction
 * that used to live alongside has been removed; the runtime
 * dictionary is produced by `kapi-react compile` from a translated
 * .klz, not directly by extract.
 */

import { describe, expect, it } from 'vitest';

import type {
  Block,
  Document,
  PlaceholderRun,
  PluralRunWrapper,
  SelectRunWrapper,
  TextRun,
} from '@neokapi/kapi-format';

import { extractDocument } from '../src/extract/index.ts';
import { hashKey } from '../src/plugin/hash.ts';

function extract(code: string, filename = 'Test.tsx', opts = {}): Document {
  const doc = extractDocument(code, { filename, ...opts });
  if (!doc) throw new Error('expected a Document, got null');
  return doc;
}

function onlyBlock(doc: Document): Block {
  expect(doc.blocks, 'expected exactly one block').toHaveLength(1);
  return doc.blocks[0];
}

function textRun(run: unknown): TextRun {
  if (!run || typeof run !== 'object' || !('text' in run)) {
    throw new Error(`expected TextRun, got ${JSON.stringify(run)}`);
  }
  return run as TextRun;
}

function phRun(run: unknown): PlaceholderRun {
  if (!run || typeof run !== 'object' || !('ph' in run)) {
    throw new Error(`expected PlaceholderRun, got ${JSON.stringify(run)}`);
  }
  return run as PlaceholderRun;
}

describe('extractDocument — element blocks', () => {
  it('emits one block with a single TextRun for `<h1>Hello World</h1>`', () => {
    const block = onlyBlock(extract('<h1>Hello World</h1>'));
    expect(block.type).toBe('jsx:element');
    expect(block.source).toHaveLength(1);
    expect(textRun(block.source[0]).text).toBe('Hello World');
    expect(block.properties.jsxPath).toBe('h1');
    expect(block.properties.element).toBe('h1');
    expect(block.hash).toBe(hashKey('Hello World', 'h1'));
  });

  it('routes expression containers to jsx:var placeholders', () => {
    const block = onlyBlock(extract('<h1>Hello, {name}!</h1>'));
    expect(block.source).toHaveLength(3);
    expect(textRun(block.source[0]).text).toBe('Hello, ');
    const ph = phRun(block.source[1]).ph;
    expect(ph.type).toBe('jsx:var');
    expect(ph.equiv).toBe('name');
    expect(textRun(block.source[2]).text).toBe('!');
    expect(block.placeholders.find((p) => p.name === 'name')?.kind).toBe('variable');
    expect(block.hash).toBe(hashKey('Hello, {name}!', 'h1'));
  });

  it('flattens inline elements to a single jsx:element placeholder, consuming their content', () => {
    const doc = extract('<h2>Files <span>{count} matched</span></h2>');
    // Parent block carries `"Files {=m0}"` with the span as a ph.
    // The span's content is opaque at runtime (tx() splices the
    // whole React element back in), so we do NOT emit a separate
    // block for it — that would be a translator ghost entry the
    // runtime never looks up.
    expect(doc.blocks).toHaveLength(1);
    const parent = doc.blocks[0];
    expect(parent.properties.jsxPath).toBe('h2');
    expect(parent.source).toHaveLength(2);
    expect(textRun(parent.source[0]).text).toBe('Files ');
    const outerPh = phRun(parent.source[1]).ph;
    expect(outerPh.type).toBe('jsx:element');
    expect(outerPh.subType).toBe('span');
    expect(parent.hash).toBe(hashKey('Files {=m0}', 'h2'));
  });

  it('emits jsx:element placeholder for `<Icon/>`', () => {
    const block = onlyBlock(
      extract('<button>Save <Icon/></button>', 'Test.tsx', {
        componentMap: { Icon: 'span' },
      }),
    );
    const [text, ph] = block.source;
    expect(textRun(text).text).toBe('Save ');
    const icon = phRun(ph).ph;
    expect(icon.type).toBe('jsx:element');
    expect(icon.subType).toBe('span');
    expect(block.placeholders.find((p) => p.kind === 'element')).toBeTruthy();
  });

  it('skips expression-only parents, extracts the nested JSX text', () => {
    // `<button>{show && <span>Save</span>}</button>` has no
    // translator-editable text at the button level. The walker
    // extracts the inner `<span>Save</span>` as its own block so
    // "Save" still ends up in the dictionary; the outer button
    // renders as vanilla JSX at runtime.
    const doc = extract('<button>{show && <span>Save</span>}</button>');
    expect(doc.blocks.find((b) => b.properties.jsxPath === 'button')).toBeUndefined();
    const inner = doc.blocks.find((b) => b.properties.jsxPath === 'button > span');
    expect(inner).toBeTruthy();
    expect(textRun(inner?.source[0]).text).toBe('Save');
  });

  it('deduplicates placeholder equivs across a single block', () => {
    const block = onlyBlock(extract('<p>{x} and {x}</p>'));
    const equivs = block.source
      .filter((r): r is PlaceholderRun => 'ph' in r)
      .map((r) => r.ph.equiv);
    expect(equivs).toEqual(['x', 'x_2']);
  });

  it('resolves componentMap for custom components', () => {
    const block = onlyBlock(
      extract('<Button>Click</Button>', 'Test.tsx', { componentMap: { Button: 'button' } }),
    );
    expect(textRun(block.source[0]).text).toBe('Click');
    expect(block.properties.element).toBe('Button');
  });

  it('records component name from the enclosing React function', () => {
    const code = `
      function HeroSection() {
        return <h1>Welcome</h1>;
      }
    `;
    const block = onlyBlock(extract(code, 'HeroSection.tsx'));
    expect(block.properties.component).toBe('HeroSection');
  });

  it('falls back to the filename stem when no component is detected', () => {
    const block = onlyBlock(extract('<h1>Hello</h1>', 'PlainFile.tsx'));
    expect(block.properties.component).toBe('PlainFile');
  });

  it('builds a nested jsxPath for `<li><button>Save</button></li>`', () => {
    const block = onlyBlock(extract('<li><button>Save</button></li>'));
    expect(block.properties.jsxPath).toBe('li > button');
  });
});

describe('extractDocument — skip rules', () => {
  it('skips <code> content (non-translatable element)', () => {
    expect(extractDocument('<code>x = 1</code>', { filename: 'Test.tsx' })).toBeNull();
  });

  it('extracts <div> with direct text (container auto-promoted)', () => {
    // Container elements (div, section, …) classify as non-translatable
    // per W3C, but `<div>Text</div>` is too common a React idiom to
    // silently drop. Promotion kicks in when the element has direct
    // text + all-inline children.
    const doc = extractDocument('<div>text</div>', { filename: 'Test.tsx' });
    expect(doc?.blocks).toHaveLength(1);
    expect(doc?.blocks[0].source).toEqual([{ text: 'text' }]);
  });

  it('does NOT promote a container whose children include a nested block', () => {
    // When a div mixes text with block-level children, the nested
    // block gets its own Block; the outer div stays non-translatable.
    const doc = extractDocument('<div>lead<p>body</p></div>', { filename: 'Test.tsx' });
    // <p>body</p> → 1 block; the div itself is not promoted.
    expect(doc?.blocks.map((b) => b.properties.element)).toEqual(['p']);
  });

  it('still respects `translate="no"` on a container', () => {
    expect(
      extractDocument('<div translate="no">text</div>', { filename: 'Test.tsx' }),
    ).toBeNull();
  });

  it('extracts translatable props on unmapped components', () => {
    // `<PageHeader title="Translation Memories" />` is the dominant
    // page-heading pattern — title/subtitle/description/… should
    // extract without a componentMap entry.
    const doc = extractDocument(
      '<PageHeader title="Translation Memories" subtitle="Glossaries" />',
      { filename: 'Test.tsx' },
    );
    const attrBlocks = doc?.blocks.filter((b) => b.type === 'jsx:attribute') ?? [];
    const texts = attrBlocks.map((b) => (b.source[0] as { text: string }).text).sort();
    expect(texts).toEqual(['Glossaries', 'Translation Memories']);
  });

  it('respects `translate="no"`', () => {
    expect(extractDocument('<h1 translate="no">Skip</h1>', { filename: 'Test.tsx' })).toBeNull();
  });

  it('ignores unparseable source', () => {
    expect(extractDocument('<h1 broken', { filename: 'Test.tsx' })).toBeNull();
  });
});

describe('extractDocument — attribute blocks', () => {
  it('emits a jsx:attribute block for `placeholder=`', () => {
    const block = onlyBlock(extract('<input placeholder="Search..." />'));
    expect(block.type).toBe('jsx:attribute');
    expect(textRun(block.source[0]).text).toBe('Search...');
    expect(block.properties.jsxPath).toBe('input[placeholder]');
  });

  it('emits attribute blocks even when the element itself is non-translatable', () => {
    const doc = extract('<div aria-label="Menu" />');
    // <div> is a container so it has no element block, but the aria-label survives.
    expect(doc.blocks).toHaveLength(1);
    expect(doc.blocks[0].type).toBe('jsx:attribute');
  });

  it('carries the file location in properties', () => {
    const block = onlyBlock(extract('<h1>Hello</h1>', 'Src/MyComponent.tsx'));
    expect(block.properties.file).toBe('Src/MyComponent.tsx');
    expect(block.properties.line).toBeGreaterThanOrEqual(1);
  });
});

describe('extractDocument — multiple blocks', () => {
  it('emits a block per translatable element in source order', () => {
    const code = `
      <div>
        <h1>Title</h1>
        <p>Body text</p>
        <button>Save</button>
        <input placeholder="Search" />
      </div>
    `;
    const doc = extract(code, 'Page.tsx');
    expect(doc.blocks.length).toBeGreaterThanOrEqual(4);
    const paths = doc.blocks.map((b) => b.properties.jsxPath);
    expect(paths).toContain('div > h1');
    expect(paths).toContain('div > p');
    expect(paths).toContain('div > button');
    expect(paths).toContain('div > input[placeholder]');
  });

  it('deduplicates identical hashes within a file', () => {
    // Two identical h1s at the same jsxPath yield one block.
    const doc = extract('<div><h1>Hello</h1><h1>Hello</h1></div>');
    expect(doc.blocks).toHaveLength(1);
  });
});

describe('extractDocument — <Plural>', () => {
  function extractPlural(code: string) {
    const block = onlyBlock(extract(code));
    const run = block.source[0];
    if (!run || !('plural' in run)) {
      throw new Error(`expected PluralRun, got ${JSON.stringify(run)}`);
    }
    return { block, plural: (run as PluralRunWrapper).plural };
  }

  it('emits a PluralRun with typed forms for each child', () => {
    const { plural } = extractPlural(
      `<p><Plural count={count}>
        <One>1 item</One>
        <Other>{count} items</Other>
      </Plural></p>`,
    );
    expect(plural.pivot).toBe('count');
    expect(plural.forms.one).toBeTruthy();
    expect(plural.forms.other).toBeTruthy();
    expect(textRun(plural.forms.one?.[0]).text).toBe('1 item');
    const otherFirst = plural.forms.other?.[0];
    expect(otherFirst).toBeTruthy();
    expect('ph' in (otherFirst as object)).toBe(true);
  });

  it('preserves inline JSX inside a form as a typed placeholder', () => {
    const { plural } = extractPlural(
      `<p><Plural count={n}>
        <One>1 item</One>
        <Other><strong>{n}</strong> items</Other>
      </Plural></p>`,
    );
    const otherRuns = plural.forms.other ?? [];
    expect(otherRuns).toHaveLength(2);
    const ph = phRun(otherRuns[0]).ph;
    expect(ph.type).toBe('jsx:element');
    expect(ph.subType).toBe('strong');
    expect(textRun(otherRuns[1]).text).toBe(' items');
  });

  it('marks the pivot placeholder with kind `icu-pivot`', () => {
    const { block } = extractPlural(
      `<p><Plural count={items.length}>
        <One>1 item</One>
        <Other>{items.length} items</Other>
      </Plural></p>`,
    );
    const pivot = block.placeholders.find((p) => p.name === 'items.length');
    expect(pivot?.kind).toBe('icu-pivot');
    expect(pivot?.jsType).toBe('number');
  });

  it('hashes against the equivalent ICU template', () => {
    const { block } = extractPlural(
      `<p><Plural count={n}>
        <One>1 item</One>
        <Other>{n} items</Other>
      </Plural></p>`,
    );
    const icuTemplate = '{n, plural, one {1 item} other {{n} items}}';
    expect(block.hash).toBe(hashKey(icuTemplate, 'p'));
  });
});

describe('extractDocument — <Select>', () => {
  function extractSelect(code: string) {
    const block = onlyBlock(extract(code));
    const run = block.source[0];
    if (!run || !('select' in run)) {
      throw new Error(`expected SelectRun, got ${JSON.stringify(run)}`);
    }
    return { block, select: (run as SelectRunWrapper).select };
  }

  it('emits a SelectRun with cases keyed by `when`', () => {
    const { select } = extractSelect(
      `<p><Select value={role}>
        <Case when="admin">Admin</Case>
        <Case when="guest">Guest</Case>
        <Other>User</Other>
      </Select></p>`,
    );
    expect(select.pivot).toBe('role');
    expect(Object.keys(select.cases).sort()).toEqual(['admin', 'guest', 'other']);
    expect(textRun(select.cases.admin?.[0]).text).toBe('Admin');
    expect(textRun(select.cases.other?.[0]).text).toBe('User');
  });

  it('marks the pivot placeholder with kind `icu-pivot`', () => {
    const { block } = extractSelect(
      `<p><Select value={user.role}>
        <Case when="admin">Admin</Case>
        <Other>User</Other>
      </Select></p>`,
    );
    const pivot = block.placeholders.find((p) => p.name === 'user.role');
    expect(pivot?.kind).toBe('icu-pivot');
    expect(pivot?.jsType).toBe('string');
  });
});
