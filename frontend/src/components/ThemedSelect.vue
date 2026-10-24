<script setup lang="ts">
// ThemedSelect —— 完全跟随主题的下拉选择，替代浏览器原生 <select>
// （原生弹出层由系统绘制，暗色主题下会闪出白底列表）。
// 浮层复用 .history-dropdown，因此自动获得各主题的面板材质与 8bit 方角。
import { onBeforeUnmount, ref, watch } from 'vue'

export interface ThemedOption {
  name: string
  value: string
}

const props = defineProps<{
  modelValue: string
  options: ThemedOption[]
  ariaLabel?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [string] }>()

const open = ref(false)
const root = ref<HTMLElement | null>(null)

function currentName(): string {
  return props.options.find((o) => o.value === props.modelValue)?.name ?? props.options[0]?.name ?? ''
}

function toggle() {
  open.value = !open.value
}

function choose(value: string) {
  emit('update:modelValue', value)
  open.value = false
}

function onDocClick(e: MouseEvent) {
  // 点自己内部不关（交给选项自己的点击处理）；点其它下拉也会走到这里，
  // 于是同一时刻只会有一个菜单是打开的。
  if (!root.value?.contains(e.target as Node)) open.value = false
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') open.value = false
}

function unbind() {
  document.removeEventListener('click', onDocClick)
  document.removeEventListener('keydown', onKeydown)
}

// 打开时才挂全局监听；监听在本次点击派发结束后才挂上，不会立刻把自己关掉。
watch(open, (isOpen) => {
  if (isOpen) {
    document.addEventListener('click', onDocClick)
    document.addEventListener('keydown', onKeydown)
  } else {
    unbind()
  }
})

onBeforeUnmount(unbind)
</script>

<template>
  <span ref="root" class="themed-select">
    <button
      type="button"
      class="themed-select-trigger"
      :aria-label="ariaLabel"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="toggle"
    >
      <span class="themed-select-label">{{ currentName() }}</span>
      <span class="themed-select-caret" aria-hidden="true">▾</span>
    </button>
    <div v-if="open" class="history-dropdown themed-select-menu" role="listbox">
      <button
        v-for="opt in options"
        :key="opt.value"
        type="button"
        class="themed-select-option"
        :class="{ active: opt.value === modelValue }"
        role="option"
        :aria-selected="opt.value === modelValue"
        @click="choose(opt.value)"
      >{{ opt.name }}</button>
    </div>
  </span>
</template>
