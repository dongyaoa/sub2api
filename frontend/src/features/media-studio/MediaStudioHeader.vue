<template>
  <nav
    class="inline-grid shrink-0 grid-cols-2 gap-1 rounded-lg border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-900"
    role="tablist"
    :aria-label="t('mediaStudio.switcher')"
  >
    <RouterLink
      to="/image-studio"
      class="flex h-7 min-w-20 items-center justify-center gap-1.5 rounded-md px-3.5 text-[13px] font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500/30"
      :class="active === 'image'
        ? 'bg-primary-50 text-primary-700 ring-1 ring-inset ring-primary-100 dark:bg-primary-950/40 dark:text-primary-300 dark:ring-primary-900/50'
        : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white'"
      role="tab"
      :aria-selected="active === 'image'"
    >
      <Icon name="sparkles" size="sm" />
      {{ t('mediaStudio.image') }}
    </RouterLink>
    <RouterLink
      to="/video-studio"
      class="flex h-7 min-w-20 items-center justify-center gap-1.5 rounded-md px-3.5 text-[13px] font-semibold transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500/30"
      :class="active === 'video'
        ? 'bg-primary-50 text-primary-700 ring-1 ring-inset ring-primary-100 dark:bg-primary-950/40 dark:text-primary-300 dark:ring-primary-900/50'
        : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white'"
      role="tab"
      :aria-selected="active === 'video'"
    >
      <Icon name="play" size="sm" />
      {{ t('mediaStudio.video') }}
    </RouterLink>
  </nav>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  active: 'image' | 'video'
}>()

const { t } = useI18n()
</script>
