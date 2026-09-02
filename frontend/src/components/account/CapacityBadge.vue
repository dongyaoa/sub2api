<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  colorClass: string
  tooltip?: string
  current: string | number
  max: string | number
  suffix?: string
  /** Optional 0-100 fill rendered behind the compact value label. */
  progress?: number | null
}>()

const normalizedProgress = computed(() => {
  if (props.progress == null || !Number.isFinite(Number(props.progress))) return 0
  return Math.min(100, Math.max(0, Number(props.progress)))
})
</script>

<template>
  <span
    :class="[
      'relative inline-flex items-center gap-1 overflow-hidden rounded-md px-1.5 py-px text-[10px] font-medium leading-tight',
      props.colorClass
    ]"
    :title="props.tooltip"
  >
    <span
      v-if="props.progress != null"
      class="pointer-events-none absolute inset-y-0 left-0 bg-current opacity-20 transition-[width] duration-300 ease-out"
      :style="{ width: `${normalizedProgress}%` }"
    />
    <span class="relative z-10 inline-flex items-center gap-1">
      <slot />
      <span class="font-mono">{{ props.current }}</span>
      <span class="text-gray-400 dark:text-gray-500">/</span>
      <span class="font-mono">{{ props.max }}</span>
      <span v-if="props.suffix" class="text-[9px] opacity-60">{{ props.suffix }}</span>
    </span>
  </span>
</template>
