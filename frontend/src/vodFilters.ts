// vodFilters.ts — 点播筛选的纯逻辑状态机。
// 有筛选项才暴露按钮（availableFilterGroups 非空），选择值经
// ShellService.VodListFiltered 透传给站点。
import type { FilterGroup } from '../bindings/github.com/unbox/unbox/internal/provider/models'

/** 分类有非空筛选项才返回组列表 —— 这是「筛选按钮是否显示」的判定。 */
export function availableFilterGroups(cat?: { Filters?: FilterGroup[] } | null): FilterGroup[] {
  return cat?.Filters?.filter(g => (g.options ?? []).length > 0) ?? []
}

/** 每维度初始选择取第一个选项（一般即「全部」）。 */
export function initialSelections(groups: FilterGroup[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const g of groups) {
    const first = (g.options ?? [])[0]
    out[g.key] = first ? first.value : ''
  }
  return out
}

/** 单选替换一个维度，其它维度原样保留。 */
export function nextSelections(current: Record<string, string>, key: string, value: string): Record<string, string> {
  return { ...current, [key]: value }
}

/** 已应用的筛选里存在任一非空选择才算「激活」（按钮高亮/提交有意义）。 */
export function hasActiveFilter(active: Record<string, string> | null): boolean {
  return !!active && Object.values(active).some(v => v !== '')
}