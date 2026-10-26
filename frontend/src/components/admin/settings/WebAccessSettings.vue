<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import WebAccessPage from '@/components/common/WebAccessPage.vue'
import { DEFAULT_WEB_ACCESS_MESSAGE } from '@/api/webAccess'

const props = defineProps<{ enabled: boolean; message: string; ipAllowlist: string[] }>()
const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:message': [value: string]
  'update:ipAllowlist': [value: string[]]
}>()
const { t } = useI18n()
const showPreview = ref(false)
const allowlistText = computed({
  get: () => props.ipAllowlist.join('\n'),
  set: (value: string) => emit('update:ipAllowlist', value.split('\n').map(line => line.trim())),
})
const previewMessage = computed(() => props.message.trim() || DEFAULT_WEB_ACCESS_MESSAGE)
</script>

<template>
  <section class="card" data-testid="web-access-settings">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.webAccess.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.webAccess.description') }}</p>
    </div>
    <div class="space-y-5 p-6">
      <div class="flex items-start justify-between gap-5">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">{{ t('admin.settings.webAccess.enabled') }}</label>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.webAccess.enabledHint') }}</p>
        </div>
        <Toggle :model-value="enabled" :aria-label="t('admin.settings.webAccess.enabled')" @update:model-value="emit('update:enabled', $event)" />
      </div>
      <div class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-relaxed text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
        {{ t('admin.settings.webAccess.recoveryHint') }}
      </div>
      <div>
        <label for="web-access-allowlist" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.webAccess.allowlist') }}</label>
        <textarea id="web-access-allowlist" v-model="allowlistText" class="input min-h-28 font-mono text-sm" rows="4" placeholder="203.0.113.10&#10;2001:db8::/48" spellcheck="false" />
        <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.webAccess.allowlistHint') }}</p>
        <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('admin.settings.webAccess.proxyHint') }}</p>
        <details class="mt-3 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
          <summary class="cursor-pointer font-medium text-gray-600 dark:text-gray-300">{{ t('admin.settings.webAccess.localPreviewTitle') }}</summary>
          <p class="mt-2">{{ t('admin.settings.webAccess.localPreviewHint') }}</p>
        </details>
      </div>
      <div>
        <label for="web-access-message" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.settings.webAccess.message') }}</label>
        <textarea id="web-access-message" :value="message" maxlength="500" class="input min-h-24" rows="3" :placeholder="DEFAULT_WEB_ACCESS_MESSAGE" @input="emit('update:message', ($event.target as HTMLTextAreaElement).value)" />
        <div class="mt-1.5 flex items-start justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
          <span>{{ t('admin.settings.webAccess.messageHint') }}</span><span class="shrink-0">{{ message.length }}/500</span>
        </div>
      </div>
      <button type="button" class="btn btn-secondary" data-testid="web-access-preview" @click="showPreview = true">{{ t('admin.settings.webAccess.preview') }}</button>
    </div>
    <BaseDialog :show="showPreview" :title="t('admin.settings.webAccess.previewTitle')" width="normal" @close="showPreview = false">
      <WebAccessPage phase="blocked" :message="previewMessage" preview />
      <p class="mt-3 text-center text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.webAccess.previewHint') }}</p>
    </BaseDialog>
  </section>
</template>
