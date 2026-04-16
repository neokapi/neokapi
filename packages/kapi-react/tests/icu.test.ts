import { describe, it, expect } from 'vitest';
import { resolveICU } from '../src/runtime/icu.ts';

describe('ICU resolver', () => {
  describe('plural', () => {
    it('resolves "one" form', () => {
      const text = '{count, plural, one {# message} other {# messages}}';
      expect(resolveICU(text, { count: 1 }, 'en')).toBe('1 message');
    });

    it('resolves "other" form', () => {
      const text = '{count, plural, one {# message} other {# messages}}';
      expect(resolveICU(text, { count: 5 }, 'en')).toBe('5 messages');
    });

    it('resolves zero form', () => {
      const text = '{count, plural, =0 {No messages} one {# message} other {# messages}}';
      expect(resolveICU(text, { count: 0 }, 'en')).toBe('No messages');
    });

    it('handles tokens within branches', () => {
      const text = '{count, plural, one {{count} unread message from {name}} other {{count} unread messages from {name}}}';
      expect(resolveICU(text, { count: 3, name: 'Alice' }, 'en'))
        .toBe('3 unread messages from Alice');
    });
  });

  describe('select', () => {
    it('resolves matching value', () => {
      const text = '{gender, select, male {his} female {her} other {their}}';
      expect(resolveICU(text, { gender: 'male' }, 'en')).toBe('his');
      expect(resolveICU(text, { gender: 'female' }, 'en')).toBe('her');
    });

    it('falls back to other', () => {
      const text = '{gender, select, male {his} female {her} other {their}}';
      expect(resolveICU(text, { gender: 'nonbinary' }, 'en')).toBe('their');
    });

    it('handles tokens within branches', () => {
      const text = '{gender, select, male {{name} updated his profile} female {{name} updated her profile} other {{name} updated their profile}}';
      expect(resolveICU(text, { gender: 'female', name: 'Alice' }, 'en'))
        .toBe('Alice updated her profile');
    });
  });

  describe('simple substitution', () => {
    it('substitutes tokens', () => {
      const text = 'Hello, {name}!';
      expect(resolveICU(text, { name: 'World' }, 'en')).toBe('Hello, World!');
    });
  });

  describe('no params', () => {
    it('returns text as-is when no params', () => {
      expect(resolveICU('Hello', undefined, 'en')).toBe('Hello');
    });
  });
});
