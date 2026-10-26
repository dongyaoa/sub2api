<template>
  <button v-if="scrolled && !mobileOpen" type="button" class="fixed bottom-6 right-4 z-30 flex h-11 w-11 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-600 shadow-lg transition-colors hover:bg-primary-50 hover:text-primary-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700 sm:right-6" :aria-label="t('upstreamCenter.backToTop')" :title="t('upstreamCenter.backToTop')" data-testid="upstream-back-to-top" @click="backToTop"><Icon name="arrowUp" size="md" /></button>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
defineProps<{ mobileOpen?: boolean }>()
const { t } = useI18n()
const scrolled = ref(false)
function updateScroll() { scrolled.value = window.scrollY > 320 }
function backToTop() { window.scrollTo({ top: 0, behavior: window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' }) }
onMounted(() => { updateScroll(); window.addEventListener('scroll', updateScroll, { passive: true }) })
onBeforeUnmount(() => { window.removeEventListener('scroll', updateScroll) })
</script>
