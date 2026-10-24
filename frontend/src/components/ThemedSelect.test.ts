import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import ThemedSelect from './ThemedSelect.vue'

const options = [
  { name: '线路一', value: 'a' },
  { name: '线路二', value: 'b' },
]

describe('ThemedSelect', () => {
  it('shows the selected option and emits the picked value', async () => {
    const wrapper = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    expect(wrapper.find('.themed-select-label').text()).toBe('线路一')

    await wrapper.find('.themed-select-trigger').trigger('click')
    expect(wrapper.findAll('.themed-select-option')).toHaveLength(2)

    await wrapper.findAll('.themed-select-option')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')![0]).toEqual(['b'])
    expect(wrapper.find('.themed-select-menu').exists()).toBe(false)
    wrapper.unmount()
  })

  it('closes when clicking outside', async () => {
    const wrapper = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    await wrapper.find('.themed-select-trigger').trigger('click')
    expect(wrapper.find('.themed-select-menu').exists()).toBe(true)

    document.body.click()
    await nextTick()
    expect(wrapper.find('.themed-select-menu').exists()).toBe(false)
    wrapper.unmount()
  })

  it('closes the previous select when another one is opened', async () => {
    const first = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    const second = mount(ThemedSelect, { props: { modelValue: 'a', options }, attachTo: document.body })
    await first.find('.themed-select-trigger').trigger('click')
    expect(first.find('.themed-select-menu').exists()).toBe(true)

    ;(second.find('.themed-select-trigger').element as HTMLElement).click()
    await nextTick()
    expect(first.find('.themed-select-menu').exists()).toBe(false)
    first.unmount()
    second.unmount()
  })
})
