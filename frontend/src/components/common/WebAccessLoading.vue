<script setup lang="ts">
withDefaults(defineProps<{ preview?: boolean }>(), { preview: false })
</script>

<template>
  <div class="web-access-loading" :class="{ 'is-preview': preview }" role="status" aria-live="polite" aria-busy="true" data-testid="web-access-loading">
    <div class="web-access-loading-content">
      <span class="web-access-loading-mark" aria-hidden="true" />
      <span class="web-access-loading-copy">
        <span class="web-access-loading-label" lang="zh-Hant">正在檢查訪問狀態</span>
        <span class="web-access-loading-translation" lang="en">Checking availability</span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.web-access-loading { display: grid; place-items: center; min-height: 100svh; background: #f4f7f8; color: #53686e; font-family: Inter, "PingFang TC", "Microsoft JhengHei", sans-serif; -webkit-font-smoothing: antialiased; }
.web-access-loading-content { display: flex; align-items: center; gap: 12px; padding: 20px; }
.web-access-loading-mark { box-sizing: border-box; width: 18px; height: 18px; flex: 0 0 auto; border: 1.5px solid #d4e4e1; border-top-color: #188b7b; border-radius: 50%; animation: web-access-loading-spin 1.1s linear infinite; }
.web-access-loading-copy { display: flex; flex-direction: column; gap: 3px; }
.web-access-loading-label { font-size: 12px; font-weight: 500; line-height: 1.5; letter-spacing: .025em; }
.web-access-loading-translation { color: #8a9ba0; font-size: 11px; line-height: 1.4; letter-spacing: .01em; }
.is-preview { min-height: 320px; }
.dark .web-access-loading { background: #10191c; color: #ccdcde; }
.dark .web-access-loading-mark { border-color: #29423f; border-top-color: #188b7b; }
.dark .web-access-loading-translation { color: #82999e; }
@keyframes web-access-loading-spin { to { transform: rotate(360deg); } }
@media (prefers-reduced-motion: reduce) { .web-access-loading-mark { animation: none; } }
</style>
