import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import VodFilterPanel from './VodFilterPanel.vue'
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/provider/models'

const groups: FilterGroup[] = [
  { key: 'area', name: '地区', options: [{ name: '全部', value: '' }, { name: '内地', value: '内地' }] },
]

describe('VodFilterPanel', () => {
  it('按维度渲染单选下拉并回传选择', async () => {
    const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: { area: '' } } })
    expect(wrapper.text()).toContain('地区')
    const selects = wrapper.findAll('select')
    expect(selects).toHaveLength(1)
    await selects[0].setValue('内地')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([{ area: '内地' }])
  })

  it('预选值回显为当前选中', () => {
    const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: { area: '内地' } } })
    expect((wrapper.find('select').element as HTMLSelectElement).value).toBe('内地')
  })
})