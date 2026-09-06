<template>
  <div>
    <div class="flex items-center justify-between gap-4">
      <div class="min-w-0">
        <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.tlsFingerprint.label') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.openai.tlsFingerprintHint') }}
        </p>
      </div>
      <Toggle
        :model-value="enabled"
        :aria-label="t('admin.accounts.quotaControl.tlsFingerprint.label')"
        @update:model-value="$emit('update:enabled', $event)"
      />
    </div>
    <select
      v-if="enabled"
      :value="profileId ?? ''"
      class="input mt-3"
      :aria-label="t('admin.accounts.openai.tlsFingerprintProfile')"
      @change="updateProfile"
    >
      <option value="">{{ t('admin.accounts.openai.tlsFingerprintBuiltIn') }}</option>
      <option v-for="profile in profiles" :key="profile.id" :value="profile.id">{{ profile.name }}</option>
    </select>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'

defineProps<{
  enabled: boolean
  profileId: number | null
  profiles: { id: number; name: string }[]
}>()

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:profileId': [value: number | null]
}>()
const { t } = useI18n()

function updateProfile(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  emit('update:profileId', value === '' ? null : Number(value))
}
</script>
