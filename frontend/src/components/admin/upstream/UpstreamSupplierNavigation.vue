<template>
  <nav class="supplier-navigation card min-w-0 self-start overflow-hidden" :aria-label="t('upstreamCenter.quickSwitch')" data-testid="supplier-navigation">
    <div class="min-w-0 border-b border-gray-100 px-3 py-3 dark:border-dark-700">
      <div class="flex items-center gap-2 text-xs font-semibold text-gray-800 dark:text-gray-100"><Icon name="server" size="sm" class="text-primary-600 dark:text-primary-400" />{{ t('upstreamCenter.navigation.title') }}<span class="text-[10px] font-normal tabular-nums text-gray-400">{{ Math.max(0, items.length - 1) }}</span></div>
      <p class="mt-1.5 hidden text-[10px] leading-4 text-gray-400 dark:text-dark-400 xl:block">{{ t('upstreamCenter.navigation.hint') }}</p>
    </div>
    <div class="supplier-navigation-list flex min-w-0 gap-1 p-1.5 xl:block xl:space-y-1" role="tablist" :aria-orientation="wide ? 'vertical' : 'horizontal'" :aria-label="t('upstreamCenter.quickSwitch')" data-testid="supplier-quick-tabs" @keydown="navigateTabs">
      <button
        v-for="item in items"
        :id="`supplier-filter-${item.id ?? 'all'}`"
        :key="item.id ?? 'all'"
        type="button"
        role="tab"
        :aria-selected="selectedId === item.id"
        :tabindex="selectedId === item.id ? 0 : -1"
        aria-controls="supplier-results"
        :title="item.name"
        class="group flex shrink-0 items-center gap-2 rounded-lg px-2.5 py-2.5 text-left text-xs transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 xl:w-full"
        :class="selectedId === item.id ? 'bg-primary-50 font-semibold text-primary-700 dark:bg-primary-500/15 dark:text-primary-300' : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'"
        @click="emit('select', item.id)"
      >
        <span class="flex h-5 w-5 shrink-0 items-center justify-center rounded-md text-[10px] font-semibold" :class="selectedId === item.id ? 'bg-primary-100/70 text-primary-700 dark:bg-primary-500/20 dark:text-primary-300' : 'bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-dark-400'"><Icon v-if="item.id === null" name="server" size="xs" /><template v-else>{{ item.name.slice(0, 1).toUpperCase() }}</template></span>
        <span class="max-w-[140px] truncate xl:min-w-0 xl:flex-1">{{ item.name }}</span>
        <span class="shrink-0 text-[10px] font-normal tabular-nums opacity-60">{{ item.count }}</span>
      </button>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  items: { id: number | null; name: string; count: number }[]
  selectedId: number | null
}>()
const emit = defineEmits<{ select: [id: number | null] }>()
const { t } = useI18n()
const wide = ref(false)
let layoutQuery: MediaQueryList | undefined
function updateOrientation() { wide.value = !!layoutQuery?.matches }
onMounted(() => { layoutQuery = window.matchMedia?.('(min-width: 1280px)'); updateOrientation(); layoutQuery?.addEventListener?.('change', updateOrientation) })
onBeforeUnmount(() => { layoutQuery?.removeEventListener?.('change', updateOrientation) })

function navigateTabs(event: KeyboardEvent) {
  if (!['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
  const buttons = Array.from((event.currentTarget as HTMLElement).querySelectorAll<HTMLButtonElement>('[role="tab"]'))
  const index = buttons.indexOf(event.target as HTMLButtonElement)
  if (index < 0) return
  event.preventDefault()
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : (index + (['ArrowDown', 'ArrowRight'].includes(event.key) ? 1 : -1) + buttons.length) % buttons.length
  buttons[next]?.click()
  buttons[next]?.focus({ preventScroll: true })
  buttons[next]?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
}
</script>

<style scoped>
.supplier-navigation {
  position: sticky;
  top: 5rem;
  z-index: 10;
}
.supplier-navigation-list { overflow-x: auto; overscroll-behavior: contain; scrollbar-width: thin; }
@media (min-width: 1280px) {
  .supplier-navigation { display: flex; flex-direction: column; max-height: calc(100dvh - 10rem); }
  .supplier-navigation-list { min-height: 0; overflow-x: hidden; overflow-y: auto; }
}
</style>
