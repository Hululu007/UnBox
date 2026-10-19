<script setup lang="ts">
// VodFilterPanel.vue — 点播筛选弹层内容：按维度渲染下拉单选，选择实时回传。
// 模板里不做 TS 断言（Vue 模板语法不允许 `as`），类型收窄放在事件方法里。
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/provider/models'
import { nextSelections } from '../vodFilters'

const props = defineProps<{
  groups: FilterGroup[]
  modelValue: Record<string, string>
}>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

function onSelect(key: string, event: Event) {
  const value = (event.target as HTMLSelectElement).value
  emit('update:modelValue', nextSelections(props.modelValue, key, value))
}
</script>

<template>
  <div class="vod-filter-panel">
    <label v-for="g in groups" :key="g.key" class="vod-filter-dim">
      <span class="vod-filter-name">{{ g.name }}</span>
      <select :value="modelValue[g.key] ?? (g.options ?? [])[0]?.value ?? ''" @change="onSelect(g.key, $event)">
        <option v-for="opt in g.options" :key="opt.value" :value="opt.value">{{ opt.name }}</option>
      </select>
    </label>
  </div>
</template>
