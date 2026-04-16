import { describe, it, expect, beforeEach } from 'vitest';
import { t, tx, setTranslations } from '../src/runtime/index.ts';

beforeEach(() => {
  setTranslations('', {});
});

describe('t() — string translations', () => {
  it('returns fallback when no translation', () => {
    expect(t('hash1', 'Hello')).toBe('Hello');
  });

  it('returns translation when available', () => {
    setTranslations('de', { hash1: 'Hallo' });
    expect(t('hash1', 'Hello')).toBe('Hallo');
  });

  it('substitutes params', () => {
    setTranslations('de', { hash1: 'Hallo, {name}!' });
    expect(t('hash1', 'Hello, {name}!', { name: 'Alice' })).toBe('Hallo, Alice!');
  });

  it('resolves ICU plurals', () => {
    setTranslations('de', {
      hash1: '{count, plural, one {{count} Nachricht} other {{count} Nachrichten}}',
    });
    expect(t('hash1', '{count} messages', { count: 1 })).toBe('1 Nachricht');
    expect(t('hash1', '{count} messages', { count: 5 })).toBe('5 Nachrichten');
  });
});

describe('tx() — rich JSX translations', () => {
  it('returns string when no element tokens', () => {
    const result = tx('hash1', 'Hello', {});
    expect(result).toBe('Hello');
  });

  it('returns string when translation has no element tokens', () => {
    setTranslations('de', { hash1: 'Hallo' });
    const result = tx('hash1', 'Hello', {});
    expect(result).toBe('Hallo');
  });

  it('interleaves elements with text', () => {
    const link = '<a>here</a>'; // simplified — in real usage this is a ReactNode
    const result = tx('hash1', 'Click {=m0} to continue.', { '=m0': link });
    // Result should be a React element (span) containing the interleaved parts
    expect(result).not.toBe('Click {=m0} to continue.');
    expect(typeof result).toBe('object'); // ReactNode, not string
  });

  it('uses translated text with elements', () => {
    setTranslations('de', { hash1: 'Klicken Sie {=m0}, um fortzufahren.' });
    const link = '<a>hier</a>';
    const result = tx('hash1', 'Click {=m0} to continue.', { '=m0': link });
    expect(typeof result).toBe('object');
  });

  it('returns string when elements are not in the translation', () => {
    setTranslations('de', { hash1: 'Einfacher Text' });
    const result = tx('hash1', 'Simple text', { '=m0': '<b>bold</b>' });
    // No {=m0} in translation → returns plain string
    expect(result).toBe('Einfacher Text');
  });

  it('substitutes string params alongside elements', () => {
    setTranslations('de', { hash1: '{name} klickt {=m0}' });
    const link = '<a>hier</a>';
    const result = tx('hash1', '{name} clicks {=m0}', { '=m0': link }, { name: 'Alice' });
    expect(typeof result).toBe('object');
  });
});
