// 语料完整性。回落机制保证「没翻的会落成英文」，所以中文缺一条不会显示成空白——
// 它会显示成英文，夹在一片中文里，比空白更难发现。这里把两门语言的 admin 段逐键对。
import { describe, it, expect } from 'vitest'
import en from '../../internal/api/web/locales/en.json'
import zh from '../../internal/api/web/locales/zh.json'

const flat = (o, p = '') => Object.entries(o).flatMap(([k, v]) => (typeof v === 'string' ? [[p + k, v]] : flat(v, p + k + '.')))

describe('locales/admin', () => {
  const E = Object.fromEntries(flat(en.admin)), Z = Object.fromEntries(flat(zh.admin))
  it('every English key is non-empty', () => {
    for (const [k, v] of Object.entries(E)) expect(v, k).not.toBe('')
  })
  it('Chinese has every English key, and nothing else', () => {
    expect(Object.keys(Z).sort()).toEqual(Object.keys(E).sort())
    for (const [k, v] of Object.entries(Z)) expect(v, k).not.toBe('')
  })
  it('placeholders match between languages', () => {
    for (const k of Object.keys(E)) {
      const count = (s) => (s.match(/%[sd]/g) || []).length
      expect(count(Z[k]), k).toBe(count(E[k]))
    }
  })
})
