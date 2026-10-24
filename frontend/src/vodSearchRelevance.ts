// vodSearchRelevance.ts — 全站搜索结果的本地相关度排序与「强 / 弱相关」分档。
//
// 站点后端的模糊搜索（常见的 maccms 系把「忍者神龟第二季」拆成多个词做 OR
// 匹配）会混进大量只沾到其中一两个词的片子。UnBox 不修改发给站点的关键词
// （改了反而可能搜不到正确结果），而是在聚合后按标题与查询串的「最长连续字符
// 子串」打分：完整命中 / 长片段命中 → 强相关排前面，只沾孤立短词的 → 弱相关
// 沉底，中间用一条分界线隔开，让用户一眼看出哪批才是真正对上的。

/**
 * 返回查询串 query 在标题 title 中作为连续子串出现的最长字符数。
 *
 * CJK 查询没有可靠的白金分词，连续字符子串长度是最稳的相关度信号：
 * 「忍者神龟第二季」在《忍者神龟第二季》里命中 7 个、在《忍者神龟国语版》里
 * 命中 4 个、在《喜羊羊第二季》里只命中 3 个。拉丁字符按小写比较。
 */
export function longestQuerySubstring(title: string, query: string): number {
  const t = title.toLowerCase()
  const q = query.toLowerCase()
  if (!q) return 0
  for (let run = Math.min(q.length, t.length); run >= 1; run--) {
    for (let i = 0; i + run <= q.length; i++) {
      if (t.includes(q.slice(i, i + run))) return run
    }
  }
  return 0
}

/**
 * 强相关的连续命中门槛：至少命中查询的一半（向上取整），且不小于 2 个字符。
 *
 * 短查询（如「斗罗」）整体命中就算强相关；超短查询（1 个字符）无法达到门槛，
 * 这种纯模糊搜索不分档。用「连续」而不是「各自出现」作门槛，正是为了把
 * 「第二季」这类孤立短词挡在弱相关一侧。
 */
export function strongQueryThreshold(query: string): number {
  if (!query) return 0
  return Math.max(Math.ceil(query.length / 2), 2)
}

export interface SearchRelevance {
  /** 标题与查询串的最长连续字符子串长度（0 = 无交集）。 */
  run: number
  /** 标题与查询主干（去掉季数后缀）的最长连续字符子串长度。 */
  stemRun: number
  /** 是否强相关：完整命中整串，或主干命中达到强相关门槛。 */
  strong: boolean
}

// seasonSuffixRe 匹配查询串末尾的通用季数/部数/话数标记，
// 如「第一季」「第2季」「第二部」「第 1 集」。这类后缀是弱区分度信号——
// 「海贼王第一季」也能和「游戏王第一季」共享「王第一季」4 个连续字符，
// 单靠连续长度会把其它剧集的季数误判成强相关。
const seasonSuffixRe = /\s*第\s*[一二三四五六七八九十百零〇0-9]+\s*(?:季|部|话|集|篇)\s*$/

/** 去掉查询串末尾的季数标记，取标题主干（如「游戏王第一季」→「游戏王」）。 */
export function queryStem(query: string): string {
  return query.replace(seasonSuffixRe, '')
}

/** 计算单条标题对查询的相关度。 */
export function searchRelevance(title: string, query: string): SearchRelevance {
  const run = longestQuerySubstring(title, query)
  // 强相关以「标题主干命中」为准：完整命中整串，或主干（去掉季数后缀）的连续
  // 命中达到门槛。只沾「第一季」这类通用后缀的其它剧集进不了强相关档。
  const stem = queryStem(query)
  const stemRun = stem ? longestQuerySubstring(title, stem) : run
  const strong = run >= query.length || (stem !== '' && stemRun >= strongQueryThreshold(stem))
  return { run, stemRun, strong }
}

/**
 * 稳定的相关度排序：强相关整体在前，组内按连续命中长度降序，同分保持原顺序。
 * 只按 `Title` 打分，不感知站点——站点先后顺序在同一档内不受影响。
 */
export function sortVodSearchResults<T extends { Title: string }>(items: readonly T[], query: string): T[] {
  const scored = items.map(item => ({ item, rel: searchRelevance(item.Title, query) }))
  // 稳定排序的关键：只用「降序比较」返回 0/±1，不用任何依赖原始下标的技巧，
  // 引擎（V8 / Node / 浏览器）的稳定 sort 会保住同分值之间的输入顺序。
  scored.sort((a, b) => {
    if (a.rel.strong !== b.rel.strong) return a.rel.strong ? -1 : 1
    return b.rel.run - a.rel.run
  })
  return scored.map(entry => entry.item)
}

/**
 * 排序后开头连续强相关的条数，用作前端「强弱分界线」的落点下标。
 * 强相关都被排在最前，因此这是第一个弱相关的下标。
 */
export function countLeadingStrong<T extends { Title: string }>(sorted: readonly T[], query: string): number {
  let n = 0
  for (const item of sorted) {
    if (!searchRelevance(item.Title, query).strong) break
    n++
  }
  return n
}