import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import VodFilterPanel from './VodFilterPanel.vue'
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/provider/models'

const groups: FilterGroup[] = [
  { key: 'area', name: '地区', options: [{ name: '全部', value: '' }, { name: '内地', value: '内地' }] },
]

describe('VodFilterPanel', () => {
  it('按维度渲染主题下拉并回传选择', async () => {
    const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: { area: '' } } })
    expect(wrapper.text()).toContain('地区')
    expect(wrapper.findAll('select')).toHaveLength(0)
    expect(wrapper.find('.themed-select-label').text()).toBe('全部')

    await wrapper.find('.themed-select-trigger').trigger('click')
    const options = wrapper.findAll('.themed-select-option')
    expect(options.map(o => o.text())).toEqual(['全部', '内地'])
    await options[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual([{ area: '内地' }])
  })

  it('预选值回显为当前选中', () => {
    const wrapper = mount(VodFilterPanel, { props: { groups, modelValue: { area: '内地' } } })
    expect(wrapper.find('.themed-select-label').text()).toBe('内地')
  })
})
