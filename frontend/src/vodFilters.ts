// vodFilters.ts — 点播筛选的纯逻辑状态机。
// 有筛选项才暴露按钮（availableFilterGroups 非空），选择值经
// ShellService.VodListFiltered 透传给站点。
import type { FilterGroup } from '../bindings/github.com/unbox/unbox/internal/provider/models'

/** 分类有非空筛选项才返回组列表 —— 这是「筛选按钮是否显示」的判定。
 *  形参按生成的 Section 模型取小写 filters 字段（可为 null）。 */
export function availableFilterGroups(cat?: { filters?: FilterGroup[] | null } | null): FilterGroup[] {
  return cat?.filters?.filter(g => (g.options ?? []).length > 0) ?? []
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

/** 用户点「确定」后提交的筛选值：始终是整份选择，包含空值「全部」。
 *  只有从未应用过筛选时才是 null —— 脚本据此区分「选了全部」与「没有筛选」，
 *  两者在 drpy2 里分别对应 extend.area === '' 和 extend 里没有 area 键。 */
export function appliedFilter(draft: Record<string, string>): Record<string, string> {
  return { ...draft }
}
