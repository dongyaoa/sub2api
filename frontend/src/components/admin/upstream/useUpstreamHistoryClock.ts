import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

// Each cadence has one shared timer. Paused strips never depend on the seconds
// clock, so an active monitor does not make every paused card render each second.
function createClock(interval: number) {
  const now = ref(Date.now())
  let subscribers = 0
  let timer: ReturnType<typeof setInterval> | undefined

  function stop() { clearInterval(timer); timer = undefined }
  function syncVisibility() {
    stop()
    if (!subscribers || document.hidden) return
    now.value = Date.now()
    timer = setInterval(() => { now.value = Date.now() }, interval)
  }

  return {
    now,
    refreshIfUnused() { if (!subscribers) now.value = Date.now() },
    subscribe() {
      subscribers++
      if (subscribers !== 1) return
      document.addEventListener('visibilitychange', syncVisibility)
      syncVisibility()
    },
    unsubscribe() {
      subscribers--
      if (subscribers) return
      stop()
      document.removeEventListener('visibilitychange', syncVisibility)
    },
  }
}

const secondsClock = createClock(1000)
const minutesClock = createClock(60_000)

export function useUpstreamHistoryClock(enabled: () => boolean) {
  const currentClock = () => enabled() ? secondsClock : minutesClock
  let subscribedClock = currentClock()
  subscribedClock.refreshIfUnused()
  let mounted = false
  onMounted(() => {
    mounted = true
    subscribedClock = currentClock()
    subscribedClock.subscribe()
  })
  watch(enabled, () => {
    if (!mounted) return
    subscribedClock.unsubscribe()
    subscribedClock = currentClock()
    subscribedClock.subscribe()
  })
  onBeforeUnmount(() => {
    if (!mounted) return
    subscribedClock.unsubscribe()
    mounted = false
  })
  return computed(() => currentClock().now.value)
}
