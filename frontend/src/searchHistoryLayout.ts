// searchHistoryLayout.ts — 搜索历史下拉框的宽度计算。
// 收起态的内联区是单行、溢出隐藏：末尾常有一个被裁掉一半的历史词。
// 下拉框要贴住第一个历史词的开头，并在最后一个「完整显示」的历史词结尾收口，
// 不能把被裁掉的半个词的宽度算进去。

export interface InlineBox {
  left: number
  width: number
}

/** 由内联区及其子项（相对同一 offsetParent 的 offsetLeft/offsetWidth）算出下拉框位置。 */
export function searchHistoryDropdownBox(items: InlineBox[], inline: InlineBox): InlineBox {
  const limit = inline.left + inline.width
  let right = 0
  for (const item of items) {
    const end = item.left + item.width
    if (end <= limit) right = end
  }
  // 一个完整词都放不下（首词就超宽）时退回内联区整宽，至少不让下拉框塌成 0。
  if (right <= inline.left) return { left: inline.left, width: inline.width }
  return { left: inline.left, width: right - inline.left }
}
