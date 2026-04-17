import { describe, it, expect, beforeEach } from 'vitest';
import { __t, __tx, setTranslations, t } from '../src/runtime/index.ts';

beforeEach(() => {
  setTranslations('', {});
});

describe('__t() — hash-based string lookup', () => {
  it('returns fallback when no translation', () => {
    expect(__t('hash1', 'Hello')).toBe('Hello');
  });

  it('returns translation when available', () => {
    setTranslations('de', { hash1: 'Hallo' });
    expect(__t('hash1', 'Hello')).toBe('Hallo');
  });

  it('substitutes params', () => {
    setTranslations('de', { hash1: 'Hallo, {name}!' });
    expect(__t('hash1', 'Hello, {name}!', { name: 'Alice' })).toBe('Hallo, Alice!');
  });

  it('resolves ICU plurals', () => {
    setTranslations('de', {
      hash1: '{count, plural, one {{count} Nachricht} other {{count} Nachrichten}}',
    });
    expect(__t('hash1', '{count} messages', { count: 1 })).toBe('1 Nachricht');
    expect(__t('hash1', '{count} messages', { count: 5 })).toBe('5 Nachrichten');
  });
});

describe('__tx() — hash-based JSX lookup', () => {
  it('returns string when no element tokens', () => {
    const result = __tx('hash1', 'Hello', {});
    expect(result).toBe('Hello');
  });

  it('returns string when translation has no element tokens', () => {
    setTranslations('de', { hash1: 'Hallo' });
    const result = __tx('hash1', 'Hello', {});
    expect(result).toBe('Hallo');
  });

  it('interleaves elements with text', () => {
    const link = '<a>here</a>';
    const result = __tx('hash1', 'Click {=m0} to continue.', { '=m0': link });
    expect(result).not.toBe('Click {=m0} to continue.');
    expect(typeof result).toBe('object');
  });

  it('uses translated text with elements', () => {
    setTranslations('de', { hash1: 'Klicken Sie {=m0}, um fortzufahren.' });
    const link = '<a>hier</a>';
    const result = __tx('hash1', 'Click {=m0} to continue.', { '=m0': link });
    expect(typeof result).toBe('object');
  });

  it('returns string when elements are not in the translation', () => {
    setTranslations('de', { hash1: 'Einfacher Text' });
    const result = __tx('hash1', 'Simple text', { '=m0': '<b>bold</b>' });
    expect(result).toBe('Einfacher Text');
  });

  it('substitutes string params alongside elements', () => {
    setTranslations('de', { hash1: '{name} klickt {=m0}' });
    const link = '<a>hier</a>';
    const result = __tx('hash1', '{name} clicks {=m0}', { '=m0': link }, { name: 'Alice' });
    expect(typeof result).toBe('object');
  });
});

describe('t() — user-facing escape hatch (dev-mode fallback)', () => {
  it('returns the source text verbatim', () => {
    expect(t('Hello')).toBe('Hello');
  });

  it('substitutes params in the source text', () => {
    expect(t('Hello, {name}!', { name: 'Alice' })).toBe('Hello, Alice!');
  });

  it('ignores the runtime dict (plugin rewrites to __t)', () => {
    // Source text returns verbatim even when a hash-keyed entry
    // exists for the same text — the plugin is the only thing
    // that knows the hash to look up.
    setTranslations('de', { someHash: 'Hallo' });
    expect(t('Hello')).toBe('Hello');
  });
});
