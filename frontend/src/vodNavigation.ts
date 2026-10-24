import { episodePageIndex } from './episodes'

export type VodView = 'list' | 'search' | 'detail'
export type VodDetailOrigin = 'home' | 'search' | 'list' | 'favorites'
// 搜索结果缓存只服务「详情页返回时恢复上次结果」。这是 TTL 的默认回退值
// （30 分钟），实际值由用户在设置页选择并经 createVodSearchCache 传入。
export const VOD_SEARCH_CACHE_TTL = 30 * 60 * 1000

export function nextVodSearchRequest(previous: number): number {
  return previous + 1
}

export function nextVodCategoryRequest(previous: number): number {
  return previous + 1
}

export function isCurrentVodCategoryRequest(request: number, current: number): boolean {
  return request === current
}

export interface VodSearchCache<T> {
  query: string
  items: T[]
  expiresAt: number
}

export function vodBackTarget(origin: VodDetailOrigin): Exclude<VodView, 'detail'> | 'home' | 'favorites' {
  return origin
}

export function createVodSearchCache<T>(query: string, items: T[], now = Date.now(), ttl = VOD_SEARCH_CACHE_TTL): VodSearchCache<T> {
  return { query, items: [...items], expiresAt: now + ttl }
}

export function isVodSearchCacheValid<T>(cache: VodSearchCache<T> | null, now = Date.now()): boolean {
  return !!cache && cache.expiresAt > now
}

export function vodSearchQueryForReturn<T>(cache: VodSearchCache<T> | null, currentQuery: string): string {
  return cache?.query || currentQuery
}

export function upsertVodSearchHistory(history: string[], query: string, limit = 30): string[] {
  const value = query.trim()
  if (!value) return history.slice(0, limit)
  return [value, ...history.filter(item => item !== value)].slice(0, limit)
}

export function removeVodSearchHistory(history: string[], query: string): string[] {
  return history.filter(item => item !== query)
}

export function removeVodHistory<T extends { Site: string; VodID: string }>(items: T[], site: string, vodID: string): T[] {
  return items.filter(item => item.Site !== site || item.VodID !== vodID)
}

export function removeVodFavorite<T extends { Site: string; VodID: string }>(items: T[], site: string, vodID: string): T[] {
  return items.filter(item => item.Site !== site || item.VodID !== vodID)
}

export function shouldShowVodNoResults(inputQuery: string, completedQuery: string, searching: boolean, resultCount: number): boolean {
  const query = inputQuery.trim()
  return query !== '' && query === completedQuery && !searching && resultCount === 0
}

export function pickResumeSource(historySource: string, available: readonly string[] | null | undefined): string {
  return historySource && available?.includes(historySource) ? historySource : available?.[0] ?? ''
}

// pickResumeSeek 把延迟续播目标的进度套用到用户点击的那一集；未命中返回 0（从头播）。
export function pickResumeSeek(target: { EpID: string; Progress: number } | null, epID: string): number {
  return target && target.EpID === epID ? target.Progress : 0
}

export interface VodResumeView {
  source: string
  episodeID: string
  page: number
}

// vodResumeView 解析「接着上次看」应呈现的视图：线路、高亮的集数、该集所在分页。
// 线路失效时回落到首条可用线路；无记录时不高亮任何集。
export function vodResumeView(
  detail: { Sources?: readonly string[] | null; Episodes?: readonly { ID: string; Source: string }[] | null } | null,
  history: { EpID: string; Source: string } | null,
): VodResumeView {
  const source = pickResumeSource(history?.Source ?? '', detail?.Sources)
  const episodeID = history?.EpID ?? ''
  const episodes = (detail?.Episodes ?? []).filter(ep => ep.Source === source)
  return { source, episodeID, page: episodePageIndex(episodes, episodeID) }
}

export function resolveVodSelection<T extends { ID: string; Line?: string }>(sites: readonly T[], preferredSite: string): { site: string; line: string } {
  const target = sites.find(site => site.ID === preferredSite) ?? sites[0]
  return target ? { site: target.ID, line: target.Line ?? '' } : { site: '', line: '' }
}
