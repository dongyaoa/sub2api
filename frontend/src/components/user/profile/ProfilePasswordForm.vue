<template>
  <div :class="props.compact ? 'min-w-0 py-4 first:pt-0 last:pb-0' : props.embedded ? 'space-y-4' : 'card'">
    <div v-if="props.compact" class="flex items-start gap-3" data-testid="profile-password-summary">
      <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-300">
        <Icon name="lock" size="sm" />
      </div>
      <div class="min-w-0 flex-1">
        <h3 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('profile.changePassword') }}</h3>
        <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ t('profile.passwordSummary') }}</p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm shrink-0" data-testid="profile-password-manage" @click="showDialog = true">
        {{ t('common.edit') }}
      </button>
    </div>
    <div
      v-if="!props.embedded && !props.compact"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.changePassword') }}
      </h2>
    </div>
    <component
      :is="props.compact ? BaseDialog : 'div'"
      v-bind="props.compact ? { show: showDialog, title: t('profile.changePassword'), width: 'narrow' as const } : {}"
      :class="props.embedded || props.compact ? '' : 'px-6 py-6'"
      @close="showDialog = false"
    >
      <form @submit.prevent="handleChangePassword" class="space-y-4">
        <div v-if="props.embedded && !props.compact">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.changePassword') }}
          </p>
        </div>
        <div>
          <label for="old_password" class="input-label">
            {{ t('profile.currentPassword') }}
          </label>
          <input
            id="old_password"
            v-model="form.old_password"
            type="password"
            required
            autocomplete="current-password"
            class="input"
          />
        </div>

        <div>
          <label for="new_password" class="input-label">
            {{ t('profile.newPassword') }}
          </label>
          <input
            id="new_password"
            v-model="form.new_password"
            type="password"
            required
            autocomplete="new-password"
            class="input"
          />
          <p class="input-hint">
            {{ t('profile.passwordHint') }}
          </p>
        </div>

        <div>
          <label for="confirm_password" class="input-label">
            {{ t('profile.confirmNewPassword') }}
          </label>
          <input
            id="confirm_password"
            v-model="form.confirm_password"
            type="password"
            required
            autocomplete="new-password"
            class="input"
          />
        </div>

        <div class="flex justify-end pt-4">
          <button type="submit" :disabled="loading" class="btn btn-primary">
            {{ loading ? t('profile.changingPassword') : t('profile.changePasswordButton') }}
          </button>
        </div>
      </form>
    </component>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const props = withDefaults(defineProps<{
  embedded?: boolean
  compact?: boolean
}>(), {
  embedded: false,
  compact: false,
})

const loading = ref(false)
const showDialog = ref(false)
const form = ref({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

const handleChangePassword = async () => {
  if (form.value.new_password !== form.value.confirm_password) {
    appStore.showError(t('profile.passwordsNotMatch'))
    return
  }

  if (form.value.new_password.length < 8) {
    appStore.showError(t('profile.passwordTooShort'))
    return
  }

  loading.value = true
  try {
    await userAPI.changePassword(form.value.old_password, form.value.new_password)
    form.value = { old_password: '', new_password: '', confirm_password: '' }
    showDialog.value = false
    appStore.showSuccess(t('profile.passwordChangeSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('profile.passwordChangeFailed')))
  } finally {
    loading.value = false
  }
}
</script>
