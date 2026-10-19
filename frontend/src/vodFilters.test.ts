import { describe, expect, it } from 'vitest'
import {
  availableFilterGroups,
  hasActiveFilter,
  initialSelections,
  nextSelections,
} from './vodFilters'
import type { FilterGroup } from '../bindings/github.com/unbox/unbox/internal/provider/models'

const groups: FilterGroup[] = [
  { key: 'area', name: '地区', options: [{ name: '全部', value: '' }, { name: '内地', value: '内地' }] },
  { key: 'year', name: '年份', options: [{ name: '全部', value: '' }, { name: '2023', value: '2023' }] },
]

describe('availableFilterGroups', () => {
  it('有非空筛选项的分类返回组列表', () => {
    expect(availableFilterGroups({ filters: groups })).toHaveLength(2)
  })
  it('无筛选项 / 空数组 / undefined 都返回空', () => {
    expect(availableFilterGroups({ filters: [] })).toEqual([])
    expect(availableFilterGroups({})).toEqual([])
    expect(availableFilterGroups(null)).toEqual([])
    expect(availableFilterGroups()).toEqual([])
  })
})

describe('initialSelections', () => {
  it('默认取每维度的第一个选项值', () => {
    expect(initialSelections(groups)).toEqual({ area: '', year: '' })
  })
  it('空组返回空对象', () => {
    expect(initialSelections([])).toEqual({})
  })
})

describe('nextSelections', () => {
  it('单选替换、不污染其它维度', () => {
    const s = nextSelections({ area: '', year: '' }, 'area', '内地')
    expect(s).toEqual({ area: '内地', year: '' })
  })
})

describe('hasActiveFilter', () => {
  it('有任一非空选择才算激活', () => {
    expect(hasActiveFilter({ area: '内地', year: '' })).toBe(true)
    expect(hasActiveFilter({ area: '', year: '' })).toBe(false)
    expect(hasActiveFilter(null)).toBe(false)
  })
})