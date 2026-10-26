import { watch } from 'vue'
import { webAccessGate, type createWebAccessGate } from './webAccess'

export const WEB_ACCESS_BLOCKED_TITLE = 'Exclusively for overseas users'

/** Only the real gate owns this policy. Preview components never install it. */
export function bindWebAccessTitle(gate: ReturnType<typeof createWebAccessGate> = webAccessGate) {
  let normalTitle = document.title
  let wasBlocked = false
  const blocked = () => gate.state.enabled && gate.state.phase === 'blocked'
  const enforce = () => {
    if (blocked() && document.title !== WEB_ACCESS_BLOCKED_TITLE) {
      normalTitle = document.title
      document.title = WEB_ACCESS_BLOCKED_TITLE
    }
  }
  // Locale/settings completion and route hooks can write after the app has
  // unmounted. Observe only while blocked and remember their normal title.
  const observer = new MutationObserver(enforce)
  const stopWatch = watch(blocked, value => {
    if (value) {
      if (!wasBlocked) normalTitle = document.title
      wasBlocked = true
      document.title = WEB_ACCESS_BLOCKED_TITLE
      observer.observe(document.head, { subtree: true, childList: true, characterData: true })
    } else if (wasBlocked) {
      observer.disconnect()
      wasBlocked = false
      document.title = normalTitle
    }
  }, { immediate: true, flush: 'sync' })
  return () => { stopWatch(); observer.disconnect() }
}
