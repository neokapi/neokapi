/**
 * kapi-react extract tests — the AST walker emits structured
 * Block / Run sequences per AD-045. The flat-string extraction
 * that used to live alongside has been removed; the runtime
 * dictionary is produced by `kapi-react compile` from a translated
 * .klz, not directly by extract.
 */

import { describe, expect, it } from 'vitest';

import type { Block, Document, PlaceholderRun, TextRun } from '@neokapi/kapi-format';

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

  it('flattens inline elements to a single jsx:element placeholder so hash matches the plugin transform', () => {
    const doc = extract('<h2>Files <span>{count} matched</span></h2>');
    // Parent block carries only the `{=m0}` flattened reference — the
    // span's content becomes its own nested block. This mirrors what
    // plugin/transform.ts does when emitting tx() calls.
    const parent = doc.blocks.find((b) => b.properties.jsxPath === 'h2');
    expect(parent).toBeTruthy();
    expect(parent?.source).toHaveLength(2);
    expect(textRun(parent?.source[0]).text).toBe('Files ');
    const outerPh = phRun(parent?.source[1]).ph;
    expect(outerPh.type).toBe('jsx:element');
    expect(outerPh.subType).toBe('span');
    expect(parent?.hash).toBe(hashKey('Files {=m0}', 'h2'));

    const nested = doc.blocks.find((b) => b.properties.jsxPath === 'h2 > span');
    expect(nested).toBeTruthy();
    expect(phRun(nested?.source[0]).ph.equiv).toBe('count');
    expect(nested?.hash).toBe(hashKey('{count} matched', 'h2 > span'));
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

  it('marks `{cond && <X/>}` placeholders as optional jsx:node', () => {
    const doc = extract('<button>{show && <span>Save</span>}</button>');
    // Outer `<button>` captures the conditional as a jsx:node; the
    // inner `<span>` surfaces as its own nested block because its
    // content is still translatable independently.
    const outer = doc.blocks.find((b) => b.properties.jsxPath === 'button');
    const ph = phRun(outer?.source[0]).ph;
    expect(ph.type).toBe('jsx:node');
    expect(outer?.placeholders.find((p) => p.kind === 'node')?.optional).toBe(true);
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

  it('skips <div> content (container element)', () => {
    expect(extractDocument('<div>text</div>', { filename: 'Test.tsx' })).toBeNull();
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
