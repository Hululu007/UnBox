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

describe('滚动到底上报', () => {
  function defineScroll(el: HTMLElement, scrollTop: number, scrollHeight = 1000, clientHeight = 300) {
    Object.defineProperty(el, 'scrollHeight', { configurable: true, value: scrollHeight })
    Object.defineProperty(el, 'clientHeight', { configurable: true, value: clientHeight })
    Object.defineProperty(el, 'scrollTop', { configurable: true, value: scrollTop, writable: true })
  }

  it('滑到最底上报 true，往回滚上报 false', async () => {
    const wrapper = mountList([item('a'), item('b')])
    const ul = wrapper.find('ul').element as HTMLElement

    defineScroll(ul, 690)
    await wrapper.find('ul').trigger('scroll')
    expect(wrapper.emitted('atBottomChange')![0]).toEqual([false])

    defineScroll(ul, 700)
    await wrapper.find('ul').trigger('scroll')
    expect(wrapper.emitted('atBottomChange')![1]).toEqual([true])
  })

  it('暴露 isAtBottom 供父级在内容追加后重新量取', () => {
    const wrapper = mountList([item('a'), item('b')])
    defineScroll(wrapper.find('ul').element as HTMLElement, 700)
    const vm = wrapper.vm as unknown as { isAtBottom: () => boolean }
    expect(vm.isAtBottom()).toBe(true)
  })
})

describe('scrollToIndex', () => {  it('把第 index 个内容卡滚进视口（供详情页返回时恢复上次选中的位置）', async () => {
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
      // 滚动移到绘制帧执行（重挂载后等 WebKitGTK 跑完 layout 再滚）
      await new Promise((resolve) => setTimeout(resolve, 50))
      expect(scrollIntoView).toHaveBeenCalledTimes(1)
      expect(scrollIntoView.mock.calls[0][0]).toEqual({ block: 'center' })
    } finally {
      Element.prototype.scrollIntoView = original
    }
  })
})
describe('滚动偏移恢复', () => {
  // 详情页返回时必须回到离开时的偏移，而不是把选中卡片居中：
  // 卡片原本靠边时，居中定位就是肉眼可见的「跳动」。
  it('restoreScroll 原样回到离开时的偏移', () => {
    const wrapper = mountList([item('a'), item('b'), item('c')])
    const ul = wrapper.find('ul').element as HTMLElement
    Object.defineProperty(ul, 'scrollHeight', { configurable: true, value: 2000 })
    Object.defineProperty(ul, 'clientHeight', { configurable: true, value: 300 })
    Object.defineProperty(ul, 'scrollTop', { configurable: true, value: 0, writable: true })

    ;(wrapper.vm as unknown as { restoreScroll: (top: number, index?: number) => void }).restoreScroll(640, 4)
    expect(ul.scrollTop).toBe(640)
    expect((wrapper.vm as unknown as { getScrollTop: () => number }).getScrollTop()).toBe(640)
  })
})
