import { watch, type Ref } from 'vue'
import { useIntervalFn } from '@vueuse/core'

const MIN_REFRESH_INTERVAL_MS = 5000

interface AccountListRefreshOptions {
  autoEnabled: Ref<boolean>
  autoIntervalSeconds: Ref<number>
  autoCountdown: Ref<number>
  silentUntil: Ref<number>
  lastRefreshAt: Ref<number>
  isBlocked: () => boolean
  refresh: (options: { refreshTodayStats: boolean }) => Promise<void>
}

export function useAccountListRefresh(options: AccountListRefreshOptions) {
  let refreshing = false
  const { pause, resume } = useIntervalFn(async () => {
    if (refreshing || document.hidden || options.isBlocked()) return

    const now = Date.now()
    if (now < options.silentUntil.value) {
      if (options.autoEnabled.value) {
        options.autoCountdown.value = Math.ceil((options.silentUntil.value - now) / 1000)
      }
      return
    }

    if (options.autoEnabled.value) {
      options.autoCountdown.value = Math.max(0, options.autoCountdown.value - 1)
    }
    const autoDue = options.autoEnabled.value && options.autoCountdown.value === 0
    const enoughTimeElapsed = now - options.lastRefreshAt.value >= MIN_REFRESH_INTERVAL_MS
    if (!autoDue || !enoughTimeElapsed) return

    options.autoCountdown.value = options.autoIntervalSeconds.value
    options.lastRefreshAt.value = now
    refreshing = true
    try {
      await options.refresh({ refreshTodayStats: true })
    } finally {
      refreshing = false
    }
  }, 1000, { immediate: false })

  watch(options.autoEnabled, (autoEnabled) => {
    if (autoEnabled) resume()
    else pause()
  }, { immediate: true })
}
