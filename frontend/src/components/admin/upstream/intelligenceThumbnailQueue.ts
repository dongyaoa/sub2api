import { inject, onBeforeUnmount, onMounted, provide, ref, watch, type InjectionKey, type Ref } from 'vue'

type Job = () => void
interface ThumbnailQueue {
  scrolling: Ref<boolean>
  onScroll: Job
  enqueue: (job: Job) => void
  cancel: (job: Job) => void
  observe: (element: Element, changed: (visible: boolean) => void) => Job
}
const queueKey: InjectionKey<ThumbnailQueue> = Symbol('intelligence-thumbnail-queue')

// One observer covers both axes, including horizontal clipping inside each works strip.
export function provideIntelligenceThumbnailQueue(): ThumbnailQueue {
  const jobs = new Set<Job>()
  const observed = new Map<Element, (visible: boolean) => void>()
  const scrolling = ref(false)
  let observer: IntersectionObserver | undefined
  let frame = 0
  let idle: ReturnType<typeof setTimeout> | undefined

  function schedule() {
    if (frame || scrolling.value || !jobs.size) return
    frame = requestAnimationFrame(() => {
      frame = 0
      const next = jobs.values().next().value
      if (next) {
        jobs.delete(next)
        next()
      }
      schedule()
    })
  }
  const queue: ThumbnailQueue = {
    scrolling,
    onScroll() {
      scrolling.value = true
      if (frame) cancelAnimationFrame(frame)
      frame = 0
      clearTimeout(idle)
      idle = setTimeout(() => {
        scrolling.value = false
        schedule()
      }, 140)
    },
    enqueue(job) { jobs.add(job); schedule() },
    cancel(job) { jobs.delete(job) },
    observe(element, changed) {
      if (typeof IntersectionObserver === 'undefined') {
        changed(true)
        return () => {}
      }
      if (!observer) observer = new IntersectionObserver(entries => {
        for (const entry of entries) observed.get(entry.target)?.(entry.isIntersecting && entry.intersectionRatio > 0)
      }, { threshold: [0, 0.01] })
      observed.set(element, changed)
      observer.observe(element)
      return () => {
        observed.delete(element)
        observer?.unobserve(element)
      }
    }
  }
  provide(queueKey, queue)
  onBeforeUnmount(() => {
    if (frame) cancelAnimationFrame(frame)
    clearTimeout(idle)
    observer?.disconnect()
    observed.clear()
    jobs.clear()
  })
  return queue
}

export function useIntelligenceThumbnailSlot(container: Ref<HTMLElement | null>, eligible: () => boolean, large: boolean) {
  const queue = inject(queueKey, null)
  const managed = large ? null : queue
  const visible = ref(false), admitted = ref(!managed)
  let release: ReturnType<typeof setTimeout> | undefined
  let stopObserving: Job | undefined
  const admit = () => {
    if (visible.value && eligible() && !managed?.scrolling.value) admitted.value = true
  }
  if (managed) {
    watch([visible, eligible, managed.scrolling], ([inView, active, scrolling]) => {
      managed.cancel(admit)
      if (!inView || !active) {
        // Keep the live frame when a thumbnail only briefly crosses an edge.
        if (admitted.value && !release) release = setTimeout(() => { release = undefined; admitted.value = false }, 350)
      } else {
        clearTimeout(release)
        release = undefined
        if (!scrolling && !admitted.value) managed.enqueue(admit)
      }
    })
  }
  onMounted(() => {
    const element = container.value
    if (large || !element) { visible.value = large; return }
    if (managed) {
      stopObserving = managed.observe(element, inView => { visible.value = inView })
      return
    }
    if (typeof IntersectionObserver === 'undefined') { visible.value = true; return }
    const observer = new IntersectionObserver(entries => {
      const entry = entries.find(item => item.target === element)
      if (entry) visible.value = entry.isIntersecting
    }, { rootMargin: '120px' })
    observer.observe(element)
    stopObserving = () => observer.disconnect()
  })
  onBeforeUnmount(() => { clearTimeout(release); managed?.cancel(admit); stopObserving?.() })
  return { visible, admitted, managed: Boolean(managed), scrolling: managed?.scrolling }
}
