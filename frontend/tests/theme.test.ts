import test from 'node:test';
import assert from 'node:assert/strict';
import { initialTheme } from '../src/lib/theme.ts';

test('theme honors saved dark/light preference', () => {
  assert.equal(initialTheme({ getItem: () => 'dark' }, false), 'dark');
  assert.equal(initialTheme({ getItem: () => 'light' }, true), 'light');
});
test('theme falls back to system with invalid/unavailable storage', () => {
  assert.equal(initialTheme(undefined, true), 'dark');
  assert.equal(initialTheme({ getItem: () => 'invalid' }, false), 'light');
  assert.equal(initialTheme({ getItem: () => { throw new Error('blocked'); } }, true), 'dark');
});
