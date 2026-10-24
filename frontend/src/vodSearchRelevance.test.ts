import { describe, expect, it } from 'vitest'
import {
  countLeadingStrong,
  longestQuerySubstring,
  queryStem,
  searchRelevance,
  sortVodSearchResults,
  strongQueryThreshold,
} from './vodSearchRelevance'

describe('longestQuerySubstring', () => {
  it('完整命中返回整串长度', () => {
    expect(longestQuerySubstring('忍者神龟第二季', '忍者神龟第二季')).toBe(7)
  })

  it('命中查询的前缀片段按实际连续长度算', () => {
    expect(longestQuerySubstring('忍者神龟：变种时代', '忍者神龟第二季')).toBe(4)
    expect(longestQuerySubstring('忍者神龟 第二季', '忍者神龟第二季')).toBe(4)
  })

  it('只碰上一个孤立短词时只算短词长度', () => {
    expect(longestQuerySubstring('某剧第二季', '忍者神龟第二季')).toBe(3)
  })

  it('标题与查询没有共享子串返回 0', () => {
    expect(longestQuerySubstring('喜羊羊与灰太狼', '忍者神龟第二季')).toBe(0)
  })

  it('拉丁字符大小写不敏感', () => {
    expect(longestQuerySubstring('TeenAge', 'TEEN')).toBe(4)
  })

  it('查询为空返回 0', () => {
    expect(longestQuerySubstring('任意标题', '')).toBe(0)
  })
})

describe('strongQueryThreshold', () => {
  it('长查询需要约一半的连续命中（7 字符 → 4）', () => {
    expect(strongQueryThreshold('忍者神龟第二季')).toBe(4)
  })

  it('中短查询以整串为强相关门槛（4 字符 → 2）', () => {
    expect(strongQueryThreshold('斗罗大陆')).toBe(2)
  })

  it('两个字的查询整体命中即为强相关', () => {
    expect(strongQueryThreshold('斗罗')).toBe(2)
  })

  it('单字符查询没有强相关档（阈值高于查询长度）', () => {
    expect(strongQueryThreshold('猫')).toBe(2)
  })
})

describe('searchRelevance', () => {
  const query = '忍者神龟第二季'

  it('完整标题命中为强相关', () => {
    expect(searchRelevance('忍者神龟第二季', query)).toEqual({ run: 7, stemRun: 4, strong: true })
  })

  it('命中含「忍者神龟」的主干为强相关', () => {
    expect(searchRelevance('忍者神龟国语版', query)).toEqual({ run: 4, stemRun: 4, strong: true })
  })

  it('只命中「第二季」这类孤立短词为弱相关', () => {
    expect(searchRelevance('喜羊羊第二季', query)).toEqual({ run: 3, stemRun: 0, strong: false })
  })

  it('完全无关为弱相关且 run 为 0', () => {
    expect(searchRelevance('喜羊羊与灰太狼', query)).toEqual({ run: 0, stemRun: 0, strong: false })
  })
})

describe('季数后缀与标题主干（游戏王场景回归）', () => {
  it('查询去尾季数标记取主干', () => {
    expect(queryStem('游戏王第一季')).toBe('游戏王')
    expect(queryStem('忍者神龟第二季')).toBe('忍者神龟')
    expect(queryStem('第2部')).toBe('')
    expect(queryStem('斗罗大陆')).toBe('斗罗大陆')
  })

  it('其它剧集只沾「第一季」不进强相关，尽管共享长连续片段', () => {
    // 「海贼王第一季」与「游戏王第一季」共享「王第一季」4 个连续字符，
    // 旧实现据此判强相关——主干命中后应判弱、沉到分界线以下。
    const r = searchRelevance('海贼王第一季', '游戏王第一季')
    expect(r.run).toBe(4)
    expect(r.strong).toBe(false)
  })

  it('标题主干命中为强相关（含完整串与缺季数写法）', () => {
    expect(searchRelevance('游戏王第一季', '游戏王第一季').strong).toBe(true)
    expect(searchRelevance('游戏王', '游戏王第一季').strong).toBe(true)
    expect(searchRelevance('游戏王：怪兽之决斗', '游戏王第一季').strong).toBe(true)
  })
})

describe('sortVodSearchResults', () => {
  const item = (title: string) => ({ ID: title, Title: title, Site: '' })
  const query = '忍者神龟第二季'

  it('强相关整体排在弱相关前面', () => {
    const sorted = sortVodSearchResults([
      item('喜羊羊第二季'),
      item('忍者神龟第二季'),
      item('海绵宝宝第二季'),
    ], query)
    expect(sorted.map(i => i.Title)).toEqual(['忍者神龟第二季', '喜羊羊第二季', '海绵宝宝第二季'])
  })

  it('组内按命中长度降序，同分保持原顺序（稳定排序）', () => {
    const a = item('忍者神龟第二季')      // run 7，强相关
    const b = item('忍者神龟国语版')      // run 4，强相关
    const c = item('喜羊羊第二季')        // run 3，弱相关
    const d = item('海绵宝宝第二季')      // run 3，弱相关
    const sorted = sortVodSearchResults([d, c, b, a], query)
    expect(sorted.map(i => i.Title)).toEqual(['忍者神龟第二季', '忍者神龟国语版', '海绵宝宝第二季', '喜羊羊第二季'])
  })

  it('查询为空时保持原顺序', () => {
    const input = [item('甲'), item('乙')]
    expect(sortVodSearchResults(input, '').map(i => i.Title)).toEqual(['甲', '乙'])
  })
})

describe('countLeadingStrong', () => {
  const item = (title: string) => ({ ID: title, Title: title, Site: '' })

  it('统计排序后开头连续的强相关条数', () => {
    const sorted = sortVodSearchResults([
      item('喜羊羊第二季'),
      item('忍者神龟第二季'),
      item('忍者神龟国语版'),
      item('海绵宝宝第二季'),
    ], '忍者神龟第二季')
    expect(countLeadingStrong(sorted, '忍者神龟第二季')).toBe(2)
  })

  it('全部弱相关时返回 0', () => {
    expect(countLeadingStrong([item('喜羊羊第二季'), item('海绵宝宝第二季')], '忍者神龟第二季')).toBe(0)
  })

  it('全部强相关时返回总条数', () => {
    expect(countLeadingStrong([item('忍者神龟第二季'), item('忍者神龟国语版')], '忍者神龟第二季')).toBe(2)
  })
})