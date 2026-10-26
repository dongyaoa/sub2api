<template>
  <BaseDialog :show="show" :title="t('upstreamCenter.navigation.settings')" width="narrow" motion="fade" @close="emit('close')">
    <div class="space-y-5">
      <div class="flex items-center justify-between gap-4">
        <div>
          <label for="upstream-navigation-visible" class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('upstreamCenter.navigation.show') }}</label>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('upstreamCenter.navigation.showHint') }}</p>
        </div>
        <Toggle id="upstream-navigation-visible" :model-value="visible" :aria-label="t('upstreamCenter.navigation.show')" data-testid="upstream-navigation-visible" @update:model-value="emit('update:visible', $event)" />
      </div>
      <div class="border-t border-gray-100 pt-4 dark:border-dark-700">
        <p class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ t('upstreamCenter.navigation.position') }}</p>
        <div class="mt-3 grid grid-cols-2 gap-3" role="group" :aria-label="t('upstreamCenter.navigation.position')">
          <button v-for="position in positions" :key="position" type="button" class="flex items-center justify-center gap-2 rounded-xl border px-4 py-3 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" :class="side === position ? 'border-primary-300 bg-primary-50 font-medium text-primary-700 dark:border-primary-500/50 dark:bg-primary-500/15 dark:text-primary-300' : 'border-gray-200 text-gray-500 hover:border-primary-300 hover:text-primary-600 dark:border-dark-600 dark:text-dark-300'" :aria-pressed="side === position" :data-navigation-side="position" @click="emit('update:side', position)">
            <Icon :name="position === 'left' ? 'chevronLeft' : 'chevronRight'" size="sm" />{{ t(`upstreamCenter.navigation.${position}`) }}
          </button>
        </div>
        <p class="mt-2 text-xs leading-5 text-gray-400 dark:text-dark-400">{{ t('upstreamCenter.navigation.positionHint') }}</p>
      </div>
      <p class="text-xs text-gray-400 dark:text-dark-400">{{ t('upstreamCenter.navigation.savedHint') }}</p>
    </div>
    <template #footer><button type="button" class="btn btn-primary" data-testid="upstream-settings-close" @click="emit('close')">{{ t('common.close') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'

defineProps<{ show: boolean; visible: boolean; side: 'left' | 'right' }>()
const emit = defineEmits<{ close: []; 'update:visible': [visible: boolean]; 'update:side': [side: 'left' | 'right'] }>()
const { t } = useI18n()
const positions = ['left', 'right'] as const
</script>
