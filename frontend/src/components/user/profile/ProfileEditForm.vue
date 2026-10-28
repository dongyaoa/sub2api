<template>
  <div :class="props.compact ? 'min-w-0' : props.embedded ? 'space-y-4' : 'card'">
    <div
      v-if="!props.embedded && !props.compact"
      class="border-b border-gray-100 px-6 py-4 dark:border-dark-700"
    >
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.editProfile') }}
      </h2>
    </div>
    <div :class="props.embedded || props.compact ? '' : 'px-6 py-6'">
      <form @submit.prevent="handleUpdateProfile" :class="props.compact ? 'grid grid-cols-[minmax(0,1fr)_auto] items-end gap-2' : 'space-y-4'">
        <div v-if="props.embedded && !props.compact">
          <p class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('profile.editProfile') }}
          </p>
        </div>
        <div class="min-w-0">
          <label for="username" class="input-label">
            {{ t('profile.username') }}
          </label>
          <input
            id="username"
            v-model="username"
            type="text"
            class="input"
            :placeholder="t('profile.enterUsername')"
          />
        </div>

        <div class="flex justify-end" :class="props.compact ? '' : 'pt-4'">
          <button type="submit" :disabled="loading" class="btn btn-primary" :class="props.compact ? 'h-[42px] whitespace-nowrap px-3 text-xs' : ''">
            {{ loading ? t('profile.updating') : t('profile.updateProfile') }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = withDefaults(defineProps<{
  initialUsername: string
  embedded?: boolean
  compact?: boolean
}>(), {
  embedded: false,
  compact: false,
})

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const username = ref(props.initialUsername)
let savedUsername = props.initialUsername
const loading = ref(false)

watch(() => props.initialUsername, (val) => {
  if (username.value === savedUsername) username.value = val
  savedUsername = val
})

const handleUpdateProfile = async () => {
  if (!username.value.trim()) {
    appStore.showError(t('profile.usernameRequired'))
    return
  }

  loading.value = true
  const submittedUsername = username.value
  try {
    const updatedUser = await userAPI.updateProfile({
      username: submittedUsername
    })
    if (username.value === submittedUsername) username.value = updatedUser.username
    savedUsername = updatedUser.username
    authStore.user = updatedUser
    appStore.showSuccess(t('profile.updateSuccess'))
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('profile.updateFailed')))
  } finally {
    loading.value = false
  }
}
</script>
