/**
 * Dev-mode render tests for the Plural / Select authoring components.
 * These exercise the fallback path that fires when the plugin hasn't
 * rewritten the JSX into a __tx() call — i.e., plain dev server with
 * no build-time locale resolution.
 */

import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

import {
  Case,
  Few,
  Many,
  One,
  Other,
  Plural,
  pluralKeyFor,
  Select,
  Two,
  Zero,
} from '../src/runtime/plural.tsx';

function render(node: React.ReactNode): string {
  return renderToStaticMarkup(<>{node}</>);
}

describe('<Plural>', () => {
  it('picks the `one` form for count=1 under en', () => {
    const html = render(
      <Plural count={1} locale="en">
        <Zero>empty</Zero>
        <One>one item</One>
        <Other>many items</Other>
      </Plural>,
    );
    expect(html).toBe('one item');
  });

  it('falls back to the `other` form when no form matches', () => {
    const html = render(
      <Plural count={42} locale="en">
        <One>one</One>
        <Other>many</Other>
      </Plural>,
    );
    expect(html).toBe('many');
  });

  it('honours locale-specific plural rules (en zero → other)', () => {
    // English only has {one, other}; en treats 0 as `other` by CLDR.
    const html = render(
      <Plural count={0} locale="en">
        <Zero>zero items</Zero>
        <One>one item</One>
        <Other>many</Other>
      </Plural>,
    );
    expect(html).toBe('many');
  });

  it('preserves inline JSX inside the chosen form', () => {
    const html = render(
      <Plural count={3} locale="en">
        <One>one</One>
        <Other>
          <strong>3</strong> items
        </Other>
      </Plural>,
    );
    expect(html).toBe('<strong>3</strong> items');
  });

  it('supports Arabic `few` / `many` via Intl.PluralRules', () => {
    const html = render(
      <Plural count={5} locale="ar">
        <One>one</One>
        <Two>two</Two>
        <Few>few</Few>
        <Many>many</Many>
        <Other>other</Other>
      </Plural>,
    );
    // ar CLDR: 5 → few (3-10).
    expect(html).toBe('few');
  });
});

describe('<Select>', () => {
  it('matches the Case whose `when` equals the value', () => {
    const html = render(
      <Select value="admin">
        <Case when="admin">Admin panel</Case>
        <Case when="guest">Guest view</Case>
        <Other>Default</Other>
      </Select>,
    );
    expect(html).toBe('Admin panel');
  });

  it('falls back to Other when no Case matches', () => {
    const html = render(
      <Select value="unknown">
        <Case when="a">A</Case>
        <Case when="b">B</Case>
        <Other>Fallback</Other>
      </Select>,
    );
    expect(html).toBe('Fallback');
  });

  it('returns null when no matching Case and no Other', () => {
    const html = render(
      <Select value="unknown">
        <Case when="a">A</Case>
      </Select>,
    );
    expect(html).toBe('');
  });
});

describe('pluralKeyFor', () => {
  it('returns CLDR form names for common inputs', () => {
    expect(pluralKeyFor(1, 'en')).toBe('one');
    expect(pluralKeyFor(2, 'en')).toBe('other');
    expect(pluralKeyFor(1, 'fr')).toBe('one');
    expect(pluralKeyFor(0, 'fr')).toBe('one'); // French: 0 and 1 both one.
  });
});
