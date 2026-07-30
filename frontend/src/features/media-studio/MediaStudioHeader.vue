<template>
  <header class="mb-3 border-b border-gray-200 pb-3 dark:border-dark-700">
    <div class="grid grid-cols-1 items-center gap-2 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
      <div class="min-w-0 text-center sm:text-left">
        <h1 class="truncate text-xl font-semibold text-gray-950 dark:text-white">
          {{ t('mediaStudio.title') }}
        </h1>
        <p class="mt-0.5 hidden text-xs text-gray-400 sm:block">{{ t('mediaStudio.subtitle') }}</p>
      </div>

      <nav
        class="-mt-1 grid grid-cols-2 gap-1 justify-self-center rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-900"
        role="tablist"
        :aria-label="t('mediaStudio.switcher')"
      >
        <RouterLink
          to="/image-studio"
          class="flex h-10 min-w-28 items-center justify-center gap-2 rounded-md px-4 text-sm font-semibold transition-all focus:outline-none focus:ring-2 focus:ring-primary-500/30"
          :class="active === 'image'
            ? 'bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-300'
            : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white'"
          role="tab"
          :aria-selected="active === 'image'"
        >
          <Icon name="sparkles" size="sm" />
          {{ t('mediaStudio.image') }}
        </RouterLink>
        <RouterLink
          to="/video-studio"
          class="flex h-10 min-w-28 items-center justify-center gap-2 rounded-md px-4 text-sm font-semibold transition-all focus:outline-none focus:ring-2 focus:ring-primary-500/30"
          :class="active === 'video'
            ? 'bg-primary-50 text-primary-700 dark:bg-primary-950/40 dark:text-primary-300'
            : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-white'"
          role="tab"
          :aria-selected="active === 'video'"
        >
          <Icon name="play" size="sm" />
          {{ t('mediaStudio.video') }}
        </RouterLink>
      </nav>
      <div class="hidden sm:block" aria-hidden="true" />
    </div>
  </header>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  active: 'image' | 'video'
}>()

const { t } = useI18n()
</script>
