import { describe, it, expect } from 'vitest';
import { extractStrings } from '../src/extract.ts';

describe('kapi-react extract', () => {
  it('extracts text from <h1>', () => {
    const strings = extractStrings('<h1>Hello World</h1>', 'Test.tsx');
    expect(strings).toHaveLength(1);
    expect(strings[0].text).toBe('Hello World');
    expect(strings[0].context).toBe('h1');
    expect(strings[0].hash).toBeTruthy();
  });

  it('extracts text with expressions', () => {
    const strings = extractStrings('<h1>Hello, {name}!</h1>', 'Test.tsx');
    expect(strings).toHaveLength(1);
    expect(strings[0].text).toBe('Hello, {name}!');
  });

  it('extracts placeholder attribute', () => {
    const strings = extractStrings('<input placeholder="Search..." />', 'Test.tsx');
    expect(strings).toHaveLength(1);
    expect(strings[0].text).toBe('Search...');
    expect(strings[0].context).toContain('[placeholder]');
  });

  it('does NOT extract from <code>', () => {
    const strings = extractStrings('<code>x = 1</code>', 'Test.tsx');
    expect(strings).toHaveLength(0);
  });

  it('does NOT extract from <div> (container)', () => {
    const strings = extractStrings('<div>text</div>', 'Test.tsx');
    expect(strings).toHaveLength(0);
  });

  it('respects translate="no"', () => {
    const strings = extractStrings('<h1 translate="no">Skip</h1>', 'Test.tsx');
    expect(strings).toHaveLength(0);
  });

  it('builds JSX path context', () => {
    const strings = extractStrings('<li><button>Save</button></li>', 'Test.tsx');
    expect(strings).toHaveLength(1);
    expect(strings[0].context).toBe('li > button');
  });

  it('extracts multiple strings from a page', () => {
    const code = `
      <div>
        <h1>Title</h1>
        <p>Body text</p>
        <button>Save</button>
        <input placeholder="Search" />
      </div>
    `;
    const strings = extractStrings(code, 'Page.tsx');
    expect(strings.length).toBeGreaterThanOrEqual(4);
  });

  it('includes source file in output', () => {
    const strings = extractStrings('<h1>Hello</h1>', 'MyComponent.tsx');
    expect(strings[0].src).toContain('MyComponent.tsx');
  });

  it('deduplicates param names', () => {
    const strings = extractStrings('<p>{x} and {x}</p>', 'Test.tsx');
    expect(strings[0].text).toContain('{x}');
    expect(strings[0].text).toContain('{x_2}');
  });

  it('handles componentMap', () => {
    const strings = extractStrings('<Button>Click</Button>', 'Test.tsx', {
      componentMap: { Button: 'button' },
    });
    expect(strings).toHaveLength(1);
    expect(strings[0].text).toBe('Click');
  });
});
