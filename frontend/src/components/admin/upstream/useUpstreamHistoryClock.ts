import { onBeforeUnmount, onMounted, readonly, ref } from 'vue'

// All history strips share one minute timer instead of a timer per model.
const now = ref(Date.now())
let subscribers = 0
let timer: ReturnType<typeof setInterval> | undefined

function stop() {
  clearInterval(timer)
  timer = undefined
}

function syncVisibility() {
  stop()
  if (!subscribers || document.hidden) return
  now.value = Date.now()
  timer = setInterval(() => { now.value = Date.now() }, 60_000)
}

export function useUpstreamHistoryClock() {
  // A new card must not inherit an old timestamp after the page was unmounted.
  if (!subscribers) now.value = Date.now()
  let mounted = false
  onMounted(() => {
    mounted = true
    subscribers++
    if (subscribers !== 1) return
    document.addEventListener('visibilitychange', syncVisibility)
    syncVisibility()
  })
  onBeforeUnmount(() => {
    if (!mounted) return
    subscribers--
    if (subscribers) return
    stop()
    document.removeEventListener('visibilitychange', syncVisibility)
  })
  return readonly(now)
}
