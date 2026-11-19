import { describe, expect, it } from 'vitest';
import { zhCN } from './zh-CN';
import { en } from './en';

function leafKeys(obj: Record<string, unknown>, prefix = ''): string[] {
  const keys: string[] = [];
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      keys.push(...leafKeys(value as Record<string, unknown>, path));
    } else {
      keys.push(path);
    }
  }
  return keys.sort();
}

describe('i18n dictionaries', () => {
  it('have identical key sets in zh-CN and en', () => {
    expect(leafKeys(en)).toEqual(leafKeys(zhCN as unknown as Record<string, unknown>));
  });

  it('have no empty translations', () => {
    for (const dict of [zhCN, en]) {
      for (const [key, value] of Object.entries(
        dict as unknown as Record<string, Record<string, unknown>>,
      )) {
        for (const [subKey, leaf] of Object.entries(value)) {
          expect(typeof leaf === 'string' ? leaf.trim() : leaf, `${key}.${subKey}`).not.toBe('');
        }
      }
    }
  });

  it('keep interpolation placeholders in sync', () => {
    const zh = zhCN as unknown as Record<string, Record<string, string>>;
    const enDict = en as unknown as Record<string, Record<string, string>>;
    const placeholders = (s: string) => [...s.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1]).sort();
    for (const section of Object.keys(zh)) {
      for (const key of Object.keys(zh[section])) {
        expect(placeholders(enDict[section][key]), `${section}.${key}`).toEqual(placeholders(zh[section][key]));
      }
    }
  });
});
