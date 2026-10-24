<script setup lang="ts">
// VodFilterPanel.vue — 点播筛选弹层内容：按维度渲染下拉单选，选择实时回传。
// 下拉用 ThemedSelect（浮层复用 .history-dropdown），避免原生 <select>
// 在暗色主题下弹出系统白底列表。
import type { FilterGroup } from '../../bindings/github.com/unbox/unbox/internal/provider/models'
import { nextSelections } from '../vodFilters'
import ThemedSelect from './ThemedSelect.vue'

const props = defineProps<{
  groups: FilterGroup[]
  modelValue: Record<string, string>
}>()
const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

function onPick(key: string, value: string) {
  emit('update:modelValue', nextSelections(props.modelValue, key, value))
}
</script>

<template>
  <div class="vod-filter-panel">
    <div v-for="g in groups" :key="g.key" class="vod-filter-dim">
      <span class="vod-filter-name">{{ g.name }}</span>
      <ThemedSelect
        :model-value="modelValue[g.key] ?? (g.options ?? [])[0]?.value ?? ''"
        :options="g.options ?? []"
        :aria-label="g.name"
        @update:model-value="(v) => onPick(g.key, v)"
      />
    </div>
  </div>
</template>
