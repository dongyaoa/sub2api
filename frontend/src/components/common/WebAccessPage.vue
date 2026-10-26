<script setup lang="ts">
import { DEFAULT_WEB_ACCESS_MESSAGE } from '@/api/webAccess'
import type { WebAccessPhase } from '@/services/webAccess'
import WebAccessLoading from './WebAccessLoading.vue'

withDefaults(defineProps<{
  phase?: WebAccessPhase
  message?: string
  checking?: boolean
  preview?: boolean
}>(), {
  phase: 'checking',
  message: DEFAULT_WEB_ACCESS_MESSAGE,
  checking: false,
  preview: false,
})
</script>

<template>
  <main v-if="phase === 'blocked' || phase === 'error'" class="web-access-page" :class="{ 'is-preview': preview }" aria-live="polite" :aria-busy="checking" data-testid="web-access-notice">
    <section class="web-access-panel" aria-labelledby="web-access-title">
      <div class="web-access-emblem" aria-hidden="true">
        <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" stroke-width="1.3">
          <circle cx="16" cy="16" r="11" />
          <ellipse cx="16" cy="16" rx="4.5" ry="11" />
          <path d="M5 16h22M7.2 9.4c5.4 2.3 12.2 2.3 17.6 0M7.2 22.6c5.4-2.3 12.2-2.3 17.6 0" />
        </svg>
      </div>
      <p class="web-access-eyebrow">
        <span lang="zh-Hant">{{ phase === 'blocked' ? '服務區域' : '連接暫時不可用' }}</span>
        <span class="web-access-dot" aria-hidden="true" />
        <span lang="en">{{ phase === 'blocked' ? 'SERVICE AVAILABILITY' : 'CONNECTION UNAVAILABLE' }}</span>
      </p>
      <h1 id="web-access-title" class="web-access-title" lang="zh-Hant">{{ phase === 'blocked' ? '本站僅面向海外用戶提供服務' : '暫時無法檢查訪問狀態' }}</h1>
      <p class="web-access-subtitle" lang="en">{{ phase === 'blocked' ? 'Exclusively for overseas users' : 'Unable to check access' }}</p>
      <div class="web-access-description">
        <p class="web-access-message" lang="zh-Hant">{{ phase === 'blocked' ? message : '請檢查網絡連接，稍後重試。' }}</p>
        <p class="web-access-message-en" lang="en">{{ phase === 'blocked' ? 'Web access is unavailable in mainland China.' : 'Please check your connection and try again.' }}</p>
      </div>
    </section>
  </main>
  <WebAccessLoading v-else :preview="preview" />
</template>

<style scoped>
.web-access-page {
  --access-bg: #f4f7f8;
  --access-panel: #fff;
  --access-ink: #213536;
  --access-muted: #657779;
  --access-line: #e4ecec;
  --access-accent: #188b7b;
  --access-emblem-bg: #edf8f5;
  --access-emblem-line: #d9eee7;
  box-sizing: border-box;
  display: grid;
  place-items: center;
  min-height: 100svh;
  padding: 24px;
  background: var(--access-bg);
  color: var(--access-ink);
  font-family: Inter, "PingFang TC", "Microsoft JhengHei", sans-serif;
  -webkit-font-smoothing: antialiased;
}
.web-access-page *, .web-access-page *::before, .web-access-page *::after { box-sizing: border-box; }
.web-access-panel {
  width: 100%;
  max-width: 380px;
  padding: 28px;
  border: 1px solid var(--access-line);
  border-radius: 20px;
  background: var(--access-panel);
  text-align: center;
  box-shadow: 0 18px 50px -30px rgba(24, 56, 51, .22), 0 2px 8px rgba(24, 56, 51, .025);
}
.web-access-emblem {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  margin: 0 auto 16px;
  border: 1px solid var(--access-emblem-line);
  border-radius: 50%;
  background: var(--access-emblem-bg);
  color: var(--access-accent);
}
.web-access-emblem svg { width: 30px; height: 30px; }
.web-access-eyebrow { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 7px; margin: 0; color: var(--access-muted); font-size: 10px; line-height: 1.6; }
.web-access-eyebrow [lang="en"] { font-size: 9px; letter-spacing: .09em; }
.web-access-dot { width: 3px; height: 3px; flex: 0 0 auto; border-radius: 50%; background: var(--access-accent); }
.web-access-title { margin: 13px 0 0; font-size: 20px; line-height: 1.55; font-weight: 600; letter-spacing: -.025em; text-wrap: balance; }
.web-access-subtitle { margin: 5px 0 0; color: var(--access-muted); font-size: 12px; line-height: 1.6; }
.web-access-description { margin-top: 20px; padding-top: 18px; border-top: 1px solid var(--access-line); }
.web-access-message { margin: 0; color: var(--access-muted); font-size: 12px; line-height: 1.85; white-space: pre-wrap; overflow-wrap: anywhere; }
.web-access-message-en { margin: 5px 0 0; color: var(--access-muted); font-size: 11px; line-height: 1.65; text-wrap: pretty; }
.is-preview { min-height: 320px; padding: 20px; border-radius: 12px; }
.dark .web-access-page {
  --access-bg: #10191c;
  --access-panel: #192528;
  --access-ink: #edf5f3;
  --access-muted: #a4b8b5;
  --access-line: #2a3a3d;
  --access-accent: #83d4bc;
  --access-emblem-bg: #203c36;
  --access-emblem-line: #2f5147;
}
@media (max-width: 420px) {
  .web-access-page { padding: 18px; }
  .web-access-panel { padding: 26px 22px; border-radius: 18px; }
  .web-access-title { font-size: 19px; }
  .is-preview { padding: 14px; }
}
</style>
