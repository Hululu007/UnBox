import { describe, expect, it } from 'vitest'
import { searchHistoryDropdownBox } from './searchHistoryLayout'

describe('search history dropdown box', () => {
  it('ends at the last history term that is fully visible inline', () => {
    // 内联区从 x=80 起、宽 300（右边界 380）。第 4 个词 340..440 被裁掉一半，
    // 不能算进宽度；下拉框应在第 3 个词结尾（330）收口。
    const items = [
      { left: 80, width: 60 },
      { left: 145, width: 80 },
      { left: 230, width: 100 },
      { left: 340, width: 100 },
    ]
    expect(searchHistoryDropdownBox(items, { left: 80, width: 300 })).toEqual({ left: 80, width: 250 })
  })

  it('falls back to the inline width when no term fits whole', () => {
    expect(searchHistoryDropdownBox([{ left: 80, width: 420 }], { left: 80, width: 300 })).toEqual({ left: 80, width: 300 })
  })

  it('uses the content width when every term fits', () => {
    const items = [
      { left: 80, width: 60 },
      { left: 145, width: 80 },
    ]
    expect(searchHistoryDropdownBox(items, { left: 80, width: 300 })).toEqual({ left: 80, width: 145 })
  })
})
