<script setup lang="ts">
// ContentCardList.vue — 「内容展示样式」（列表 / 卡片）的统一渲染。
//
// 首页观看记录、点播收藏、点播列表、搜索结果四处结构相同，此前各自复制了一份
// 卡片标记；归一成 ContentCardItem 后由本组件统一渲染。
//
// 列表模式统一为两行（片名 / 站点·补充），卡片模式的徽标仍按原有类名输出，
// 因此样式表里 .content-card-* 那一套不需要任何改动。
import { computed, ref } from 'vue'
import { contentCardSubtitle, type ContentCardItem } from '../contentCardItem'
import { imgError, scrollxEnter, scrollxLeave } from '../contentListDom'
import { listAtBottom } from '../vodPagination'
import type { ContentCardStyle } from '../contentCardStyle'

const props = defineProps<{
  items: ContentCardItem[]
  style: ContentCardStyle
  /** 附加到 ul 的容器类（如 home-list / favorites-list），由各页面决定。 */
  listClass?: string
  /** 可删除的列表（观看记录 / 收藏）才显示行内删除按钮与卡片删除遮罩。 */
  deletable?: boolean
  /** 当前展开了删除遮罩的条目 key；由父级持有，避免组件自己记状态。 */
  activeDeleteKey?: string | null
  /**
   * 在该下标之前插入一条「强弱相关」分界线（dividerAfter 个强相关条目之后）。
   * 为空 / 0 / 超出条数时不渲染。下标只用来定位，不参与 select 的条目标号。
   */
  dividerAfter?: number
  /** 分界线上的说明文字，如「弱相关结果」。 */
  dividerLabel?: string
}>()

// 只上报下标：各页面的数据源数组与 items 一一对应，父级用下标就能取回自己的
// 原始对象（类型不外泄），组件也不必理解任何业务字段。
const emit = defineEmits<{
  select: [index: number]
  contextmenu: [index: number, event: MouseEvent]
  remove: [index: number]
  requestDelete: []
  /** 滚动容器每次滚动都上报「是否已滑到最底」，父级据此控制底部分割线。 */
  atBottomChange: [atBottom: boolean]
}>()

// 直接挂在真实滚动容器 <ul> 上，滚动当帧就能上报，不依赖父级透传或定时轮询。
function onListScroll() {
  const el = listEl.value
  if (!el) return
  emit('atBottomChange', listAtBottom(el.scrollHeight, el.scrollTop, el.clientHeight))
}

function onContextMenu(index: number, event: MouseEvent) {
  if (!props.deletable) return
  emit('contextmenu', index, event)
}

// 点播列表页的「回到顶部」需要滚动 ul；把 DOM 元素留在组件内，只暴露这一个动作，
// 免得父级拿到组件实例却去摸它的内部节点。
const listEl = ref<HTMLElement | null>(null)

// 分界线只在「强相关条数严格介于 0 与总条数之间」时渲染：全强或全弱都不需要。
const showDivider = computed(() => {
  const at = props.dividerAfter ?? -1
  return at > 0 && at < props.items.length
})

defineExpose({
  scrollToTop: () => listEl.value?.scrollTo({ top: 0, behavior: 'smooth' }),
  /**
   * 把第 index 个内容卡滚进视口。详情页返回时用它恢复上次选中的位置：
   * 列表按块居中，比恢复 scrollTop 对网格换宽更稳。按 data-index 命中，
   * 分界线 li 占的 DOM 位不计入条目标号。
   */
  scrollToIndex: (index: number) => {
    const li = listEl.value?.querySelector(`li[data-index="${index}"]`)
    li?.scrollIntoView({ block: 'center' })
  },
  /** 当前是否已真实滑到列表最底；内容追加后父级用它重新量取。 */
  isAtBottom: () => {
    const el = listEl.value
    if (!el) return true
    return listAtBottom(el.scrollHeight, el.scrollTop, el.clientHeight)
  },
})
</script>

<template>
  <ul ref="listEl" :class="[listClass, { 'content-card-grid': style === 'grid' }]" @scroll="onListScroll">
    <template v-for="(item, index) in items" :key="item.key">
      <li v-if="showDivider && index === dividerAfter" class="content-row-divider">
        <span v-if="dividerLabel">{{ dividerLabel }}</span>
      </li>
      <li
        :data-index="index"
        class="content-row"
        :class="{ 'content-card': style === 'grid' }"
        @click="emit('select', index)"
        @contextmenu="onContextMenu(index, $event)"
      >
      <img v-if="item.logo" :src="item.logo" class="thumb" loading="lazy" referrerpolicy="no-referrer" @error="imgError" />
      <template v-if="style === 'grid'">
        <span v-if="item.site" class="content-card-badge content-card-site">{{ item.site }}</span>
        <span v-if="item.progress" class="content-card-badge content-card-progress scrollx" @mouseenter="scrollxEnter" @mouseleave="scrollxLeave">
          <span class="scrollx-inner">
            <span v-if="item.progress.ep" class="cc-ep">{{ item.progress.ep }}</span>
            <span v-if="item.progress.text" class="cc-prog">{{ item.progress.text }}</span>
          </span>
        </span>
        <span class="content-card-badge content-card-title scrollx" @mouseenter="scrollxEnter" @mouseleave="scrollxLeave"><span class="scrollx-inner">{{ item.title }}</span></span>
        <div v-if="deletable && activeDeleteKey === item.key" class="content-card-delete-mask" @click.stop>
          <button type="button" class="content-card-delete-action" @click.stop="emit('requestDelete')">删除</button>
        </div>
      </template>
      <template v-else>
        <span class="content-row-info">
          <span class="name">{{ item.title }}</span>
          <span class="sub">{{ contentCardSubtitle(item) }}</span>
        </span>
        <button v-if="deletable" class="row-delete" type="button" title="删除" @click.stop="emit('remove', index)">删除</button>
      </template>
    </li>
    </template>
  </ul>
</template>
