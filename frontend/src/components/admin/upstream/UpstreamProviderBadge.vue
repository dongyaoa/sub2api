<template>
  <span v-if="appearance" class="inline-flex shrink-0 items-center rounded-md px-1.5 py-0.5 text-[10px] font-semibold" :class="appearance.className" data-testid="upstream-provider">{{ appearance.label }}</span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { UpstreamProvider } from '@/api/admin/upstreamCenter'
const props = defineProps<{ provider: UpstreamProvider }>()
const appearance = computed(() => ({
  openai: { label: 'OpenAI', className: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300' },
  anthropic: { label: 'Anthropic', className: 'bg-orange-50 text-orange-700 dark:bg-orange-500/10 dark:text-orange-300' },
  gemini: { label: 'Gemini', className: 'bg-sky-50 text-sky-700 dark:bg-sky-500/10 dark:text-sky-300' },
})[props.provider])
</script>
