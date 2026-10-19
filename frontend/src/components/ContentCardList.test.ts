import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ContentCardList from './ContentCardList.vue'
import type { ContentCardItem } from '../contentCardItem'
import type { ContentCardStyle } from '../contentCardStyle'

function item(key: string, title = key): ContentCardItem {
  return { key, logo: 'https://example.com/poster.jpg', title, site: '', detail: '' }
}

function mountList(items: ContentCardItem[], style: ContentCardStyle = 'grid', extra: Record<string, unknown> = {}) {
  return mount(ContentCardList, {
    props: { items, style, ...extra },
  })
}

describe('强弱分界线', () => {
  it('dividerAfter 指向的条数之后渲染一条带标签的分界线', () => {
    const wrapper = mountList([item('a'), item('b'), item('c'), item('d')], 'grid', {
      dividerAfter: 2,
      dividerLabel: '弱相关结果',
    })
    const divider = wrapper.find('.content-row-divider')
    expect(divider.exists()).toBe(true)
    expect(divider.text()).toContain('弱相关结果')
    // 分界线落在第 2 个内容卡之后：前面正好两个 .content-card
    const cards = wrapper.findAll('.content-card')
    const dividerIndex = Array.from(divider.element.parentElement!.children).indexOf(divider.element)
    expect(dividerIndex).toBe(2)
    expect(cards).toHaveLength(4)
  })

  it('dividerAfter 为 0 / 未设置 / 超出条数都不渲染分界线', () => {
    const items = [item('a'), item('b')]
    expect(mountList(items, 'grid', {}).find('.content-row-divider').exists()).toBe(false)
    expect(mountList(items, 'grid', { dividerAfter: 0, dividerLabel: 'x' }).find('.content-row-divider').exists()).toBe(false)
    expect(mountList(items, 'grid', { dividerAfter: items.length, dividerLabel: 'x' }).find('.content-row-divider').exists()).toBe(false)
  })

  it('有分界线时 select 下标仍与 items 一一对应', async () => {
    const wrapper = mountList([item('a'), item('b'), item('c'), item('d')], 'grid', {
      dividerAfter: 2,
      dividerLabel: '弱相关结果',
    })
    // 分界线占一个 li，但点击第 4 个卡片应 emit 3，而不是被 DOM 位置带偏
    await wrapper.findAll('.content-card')[3].trigger('click')
    expect(wrapper.emitted('select')![0]).toEqual([3])
  })
})

describe('scrollToIndex', () => {
  it('把第 index 个内容卡滚进视口（供详情页返回时恢复上次选中的位置）', () => {
    const scrollIntoView = vi.fn()
    // jsdom 未实现 scrollIntoView；先桩掉真实实现再挂载
    const original = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = scrollIntoView
    try {
      const wrapper = mountList([item('a'), item('b'), item('c'), item('d')], 'grid', {
        dividerAfter: 2,
        dividerLabel: '弱相关结果',
      })
      ;(wrapper.vm as unknown as { scrollToIndex: (i: number) => void }).scrollToIndex(3)
      expect(scrollIntoView).toHaveBeenCalledTimes(1)
      expect(scrollIntoView.mock.calls[0][0]).toEqual({ block: 'center' })
    } finally {
      Element.prototype.scrollIntoView = original
    }
  })
})