<template>
  <div
    class="tutorial-shell relative min-h-screen overflow-hidden text-slate-950 dark:text-slate-100"
    :class="{ 'is-dark': isDark }"
    :style="tutorialShellStyle"
  >
    <div class="tutorial-noise pointer-events-none absolute inset-0"></div>
    <div class="tutorial-grid pointer-events-none absolute inset-0"></div>
    <div class="tutorial-spot tutorial-spot-a pointer-events-none absolute"></div>
    <div class="tutorial-spot tutorial-spot-b pointer-events-none absolute"></div>

    <header class="relative z-20 px-4 pt-4 sm:px-6 lg:px-10">
      <nav
        class="mx-auto flex max-w-[1280px] items-center justify-between rounded-full border border-white/65 bg-white/78 px-3 py-3 shadow-[0_20px_70px_rgba(15,23,42,0.08)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-950/72 dark:shadow-[0_24px_90px_rgba(0,0,0,0.34)]"
      >
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <div
            class="flex h-11 w-11 items-center justify-center overflow-hidden rounded-2xl border border-black/5 bg-white shadow-[0_14px_40px_rgba(15,23,42,0.12)] dark:border-white/10 dark:bg-slate-900"
          >
            <img
              v-if="siteLogo"
              :src="siteLogo"
              :alt="siteName"
              :class="['h-full w-full object-contain p-1.5', isDark ? 'site-logo-dark' : 'site-logo-light']"
            />
            <span class="brand-wordmark text-base font-semibold text-slate-950 dark:text-white">
              {{ brandInitial }}
            </span>
          </div>

          <div class="min-w-0">
            <p
              class="brand-wordmark truncate text-sm font-semibold tracking-[-0.04em] text-slate-950 dark:text-white sm:text-base"
            >
              {{ siteName }}
            </p>
            <p class="truncate text-[11px] uppercase tracking-[0.24em] text-slate-500 dark:text-slate-400">
              Tutorial
            </p>
          </div>
        </router-link>

        <div class="flex items-center gap-2 sm:gap-3">
          <LocaleSwitcher />

          <button
            @click="toggleTheme"
            class="inline-flex h-10 w-10 items-center justify-center rounded-full border border-black/5 bg-black/[0.03] text-slate-600 transition duration-300 hover:-translate-y-0.5 hover:bg-black/[0.06] hover:text-slate-950 dark:border-white/10 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:bg-slate-800/80 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link
            :to="primaryActionPath"
            class="inline-flex items-center gap-2 rounded-full bg-slate-950 px-4 py-2.5 text-sm font-medium text-white shadow-[0_18px_40px_rgba(2,6,23,0.16)] transition duration-300 hover:-translate-y-0.5 hover:bg-black dark:bg-white dark:text-slate-950 dark:hover:bg-slate-100"
          >
            {{ primaryActionLabel }}
            <Icon name="arrowRight" size="sm" />
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10 px-4 pb-16 pt-8 sm:px-6 lg:px-10">
      <div class="mx-auto flex max-w-[1280px] flex-col gap-6 lg:gap-8">
        <section class="grid gap-8 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] lg:items-end">
          <div class="max-w-2xl">
            <div
              class="inline-flex items-center gap-2 rounded-full border border-black/5 bg-white/72 px-3 py-1.5 text-[11px] font-medium uppercase tracking-[0.22em] text-slate-500 shadow-[0_12px_32px_rgba(15,23,42,0.06)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/60 dark:text-slate-300 dark:shadow-[0_12px_40px_rgba(0,0,0,0.25)]"
            >
              <span class="inline-flex h-2 w-2 rounded-full bg-teal-400"></span>
              {{ copy.eyebrow }}
            </div>

            <h1
              class="landing-display mt-6 text-[clamp(2.7rem,5vw,4.8rem)] font-semibold leading-[0.96] tracking-[-0.06em] text-slate-950 dark:text-white"
            >
              {{ copy.title }}
            </h1>

            <p class="mt-5 max-w-xl text-base leading-8 text-slate-600 dark:text-slate-300 sm:text-lg">
              {{ copy.description }}
            </p>
          </div>

          <div class="grid gap-4 sm:grid-cols-3">
            <article
              v-for="metric in tutorialMetrics"
              :key="metric.label"
              class="rounded-[28px] border border-white/60 bg-white/74 p-5 shadow-[0_20px_60px_rgba(15,23,42,0.07)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/60 dark:shadow-[0_18px_44px_rgba(0,0,0,0.26)]"
            >
              <p class="text-[11px] uppercase tracking-[0.24em] text-slate-400">{{ metric.label }}</p>
              <p class="landing-display mt-3 text-2xl font-semibold tracking-[-0.05em] text-slate-950 dark:text-white">
                {{ metric.value }}
              </p>
              <p class="mt-2 text-sm leading-7 text-slate-600 dark:text-slate-300">
                {{ metric.desc }}
              </p>
            </article>
          </div>
        </section>

        <section
          class="rounded-[34px] border border-white/60 bg-white/74 p-5 shadow-[0_24px_80px_rgba(15,23,42,0.08)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-950/62 dark:shadow-[0_24px_72px_rgba(0,0,0,0.28)] md:p-7"
        >
          <div class="flex flex-wrap gap-3">
            <button
              v-for="client in clientTabs"
              :key="client.id"
              @click="activeClient = client.id"
              class="inline-flex items-center gap-2 rounded-full border px-4 py-2 text-sm font-medium transition duration-300"
              :class="
                activeClient === client.id
                  ? 'border-slate-950 bg-slate-950 text-white shadow-[0_18px_38px_rgba(15,23,42,0.16)] dark:border-white dark:bg-white dark:text-slate-950'
                  : 'border-black/8 bg-white/72 text-slate-600 hover:-translate-y-0.5 hover:border-black/12 hover:text-slate-950 dark:border-white/10 dark:bg-slate-900/58 dark:text-slate-300 dark:hover:border-white/20 dark:hover:text-white'
              "
            >
              <Icon :name="client.icon" size="sm" />
              {{ client.label }}
            </button>
          </div>

          <div class="mt-7 grid gap-6 lg:grid-cols-[minmax(0,0.84fr)_minmax(0,1.16fr)]">
            <div
              class="rounded-[30px] border border-white/60 bg-slate-950 p-6 text-white shadow-[0_26px_80px_rgba(2,6,23,0.3)] dark:border-white/10 dark:bg-black"
            >
              <p class="text-[11px] uppercase tracking-[0.26em] text-white/45">{{ currentGuide.eyebrow }}</p>
              <h2 class="landing-display mt-3 text-2xl font-semibold tracking-[-0.05em] text-white sm:text-3xl">
                {{ currentGuide.title }}
              </h2>
              <p class="mt-4 text-sm leading-7 text-white/68">
                {{ currentGuide.summary }}
              </p>

              <div class="mt-6 space-y-3">
                <div
                  v-for="(step, index) in currentGuide.steps"
                  :key="`${currentGuide.id}-${index}`"
                  class="rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3"
                >
                  <div class="flex items-start gap-3">
                    <span
                      class="flex h-8 w-8 flex-none items-center justify-center rounded-full bg-white text-xs font-semibold text-slate-950"
                    >
                      {{ index + 1 }}
                    </span>
                    <div>
                      <p class="text-sm font-medium text-white">{{ step.title }}</p>
                      <p class="mt-1 text-sm leading-6 text-white/60">{{ step.desc }}</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="space-y-4">
              <article
                v-for="snippet in currentGuide.snippets"
                :key="snippet.title"
                class="overflow-hidden rounded-[30px] border border-white/60 bg-white/74 shadow-[0_18px_52px_rgba(15,23,42,0.08)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/60 dark:shadow-[0_18px_44px_rgba(0,0,0,0.28)]"
              >
                <div
                  class="flex flex-wrap items-center justify-between gap-3 border-b border-black/5 px-5 py-4 dark:border-white/10"
                >
                  <div>
                    <p class="text-sm font-semibold tracking-[-0.03em] text-slate-950 dark:text-white">
                      {{ snippet.title }}
                    </p>
                    <p class="mt-1 text-xs uppercase tracking-[0.22em] text-slate-400">
                      {{ snippet.path }}
                    </p>
                  </div>
                  <button
                    @click="copyText(snippet.code)"
                    class="inline-flex items-center gap-2 rounded-full border border-black/8 bg-white/80 px-3 py-1.5 text-xs font-medium text-slate-600 transition hover:text-slate-950 dark:border-white/10 dark:bg-slate-800/70 dark:text-slate-300 dark:hover:text-white"
                  >
                    <Icon name="copy" size="sm" />
                    {{ copy.copyLabel }}
                  </button>
                </div>

                <pre
                  class="overflow-x-auto bg-slate-950 px-5 py-5 text-[13px] leading-7 text-slate-200 dark:bg-[#020617]"
                ><code>{{ snippet.code }}</code></pre>
              </article>
            </div>
          </div>
        </section>

        <section class="grid gap-4 md:grid-cols-3">
          <article
            v-for="tip in currentGuide.tips"
            :key="tip.title"
            class="rounded-[28px] border border-white/60 bg-white/74 p-5 shadow-[0_18px_48px_rgba(15,23,42,0.06)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/60 dark:shadow-[0_18px_40px_rgba(0,0,0,0.24)]"
          >
            <div
              class="flex h-12 w-12 items-center justify-center rounded-2xl border border-white/60 bg-white/80 dark:border-white/10 dark:bg-slate-800/70"
            >
              <Icon :name="tip.icon" size="lg" :style="{ color: tip.color }" />
            </div>
            <h3 class="mt-4 text-lg font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
              {{ tip.title }}
            </h3>
            <p class="mt-2 text-sm leading-7 text-slate-600 dark:text-slate-300">
              {{ tip.desc }}
            </p>
          </article>
        </section>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

type ClientId = 'codex' | 'claude'

interface GuideSnippet {
  title: string
  path: string
  code: string
}

interface GuideStep {
  title: string
  desc: string
}

interface GuideTip {
  title: string
  desc: string
  icon: 'sparkles' | 'shield' | 'cpu' | 'terminal' | 'checkCircle' | 'chartBar'
  color: string
}

interface ClientGuide {
  id: ClientId
  label: string
  icon: 'terminal' | 'cpu'
  eyebrow: string
  title: string
  summary: string
  steps: GuideStep[]
  snippets: GuideSnippet[]
  tips: GuideTip[]
}

const { t, locale } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => {
  const configuredName = (appStore.cachedPublicSettings?.site_name || appStore.siteName || '').trim()
  if (!configuredName || configuredName.toLowerCase() === 'sub2api') return 'Z-API'
  return configuredName
})
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const apiBaseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || window.location.origin)
const registrationEnabled = computed(() => appStore.cachedPublicSettings?.registration_enabled ?? false)
const isDark = ref(document.documentElement.classList.contains('dark'))
const activeClient = ref<ClientId>('codex')
const isZh = computed(() => locale.value.startsWith('zh'))
const tutorialShellStyle = computed(() => ({
  background: isDark.value
    ? 'radial-gradient(circle at 12% 14%, rgba(20, 184, 166, 0.12), transparent 24%), radial-gradient(circle at 84% 18%, rgba(59, 130, 246, 0.12), transparent 24%), linear-gradient(180deg, #020617 0%, #071220 46%, #020617 100%)'
    : 'radial-gradient(circle at 12% 14%, rgba(20, 184, 166, 0.14), transparent 24%), radial-gradient(circle at 84% 18%, rgba(59, 130, 246, 0.12), transparent 24%), linear-gradient(180deg, #f8fafc 0%, #edf5ff 46%, #f8fafc 100%)'
}))

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const brandInitial = computed(() => {
  const name = siteName.value.trim()
  return name ? name.charAt(0).toUpperCase() : 'S'
})

const primaryActionPath = computed(() => {
  if (isAuthenticated.value) return dashboardPath.value
  return registrationEnabled.value ? '/register' : '/login'
})

const primaryActionLabel = computed(() => {
  if (isAuthenticated.value) return t('home.goToDashboard')
  return isZh.value ? '立即开始' : 'Get Started'
})

const copy = computed(() =>
  isZh.value
    ? {
        eyebrow: 'Client Setup',
        title: '接入文档',
        description: '为 Codex 客户端和 Claude Code 客户端准备的快速配置教程。页面尽量少废话，直接给你可复制、可落地的配置。',
        copyLabel: '复制代码'
      }
    : {
        eyebrow: 'Client Setup',
        title: 'Connection Tutorial',
        description: 'Fast setup guides for Codex and Claude Code with clean, copy-ready configs.',
        copyLabel: 'Copy'
      }
)

const tutorialMetrics = computed(() =>
  isZh.value
    ? [
        { label: '01', value: '2 Clients', desc: 'Codex 客户端与 Claude Code 客户端分别提供独立配置。' },
        { label: '02', value: 'Direct Copy', desc: '给出真实配置文件路径与代码块，照抄即可。' },
        { label: '03', value: 'Minimal Steps', desc: '把流程压到最短，尽量做到一眼看懂。' }
      ]
    : [
        { label: '01', value: '2 Clients', desc: 'Separate setup flows for Codex and Claude Code.' },
        { label: '02', value: 'Direct Copy', desc: 'Real file paths and copy-ready config blocks.' },
        { label: '03', value: 'Minimal Steps', desc: 'A shorter setup path that stays easy to scan.' }
      ]
)

const codexGuide = computed<ClientGuide>(() => {
  const baseUrl = apiBaseUrl.value || window.location.origin
  const configToml = `model_provider = "OpenAI"
model = "gpt-5.4"
review_model = "gpt-5.4"
model_reasoning_effort = "xhigh"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 1000000
model_auto_compact_token_limit = 900000

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${baseUrl}"
wire_api = "responses"
requires_openai_auth = true`

  const authJson = `{
  "OPENAI_API_KEY": "你的_API_Key"
}`

  return isZh.value
    ? {
        id: 'codex',
        label: 'Codex 客户端',
        icon: 'terminal',
        eyebrow: 'Codex Client',
        title: 'Codex 客户端接入',
        summary: '适合 Codex 本地客户端。核心思路是把本平台当作 OpenAI Responses 兼容源，写入 `config.toml` 与 `auth.json` 后直接使用。',
        steps: [
          { title: '创建 API Key', desc: '登录后台后创建一个可用的 API Key，建议先绑定好默认分组。' },
          { title: '写入配置目录', desc: '把 `config.toml` 和 `auth.json` 放到 `~/.codex` 或 Windows 的 `%userprofile%\\.codex`。' },
          { title: '启动客户端验证', desc: '重新打开 Codex 客户端，发送一条简单消息，确认已经通过当前网关转发。' }
        ],
        snippets: [
          {
            title: 'config.toml',
            path: '~/.codex/config.toml',
            code: configToml
          },
          {
            title: 'auth.json',
            path: '~/.codex/auth.json',
            code: authJson
          }
        ],
        tips: [
          {
            title: 'Base URL 使用当前站点',
            desc: '如果后台设置了公开 API 地址，文档会自动读取；否则默认使用当前访问域名。',
            icon: 'sparkles',
            color: '#14b8a6'
          },
          {
            title: '推荐先用默认模型测试',
            desc: '先不要改动模型字段，确认接通后再按团队需求调整模型与推理强度。',
            icon: 'cpu',
            color: '#38bdf8'
          },
          {
            title: '只替换 API Key',
            desc: '最常见的接入错误是把 URL 与 Key 一起改乱，初次接入时只替换 API Key 即可。',
            icon: 'checkCircle',
            color: '#8b5cf6'
          }
        ]
      }
    : {
        id: 'codex',
        label: 'Codex Client',
        icon: 'terminal',
        eyebrow: 'Codex Client',
        title: 'Connect Codex',
        summary: 'Treat this gateway as an OpenAI Responses-compatible source. Add `config.toml` and `auth.json`, then launch Codex normally.',
        steps: [
          { title: 'Create an API key', desc: 'Generate a working key first and attach it to a usable group.' },
          { title: 'Write config files', desc: 'Place `config.toml` and `auth.json` inside `~/.codex` or `%userprofile%\\.codex` on Windows.' },
          { title: 'Launch and verify', desc: 'Restart the Codex client and send a short prompt to confirm traffic is routed here.' }
        ],
        snippets: [
          { title: 'config.toml', path: '~/.codex/config.toml', code: configToml },
          { title: 'auth.json', path: '~/.codex/auth.json', code: authJson }
        ],
        tips: [
          {
            title: 'Use the current gateway URL',
            desc: 'This page reads the public API base URL when available and falls back to the current origin.',
            icon: 'sparkles',
            color: '#14b8a6'
          },
          {
            title: 'Test with the default model first',
            desc: 'Keep the provided model fields unchanged until the first successful request goes through.',
            icon: 'cpu',
            color: '#38bdf8'
          },
          {
            title: 'Only replace the API key',
            desc: 'Most first-time issues come from changing both the URL and the key at once.',
            icon: 'checkCircle',
            color: '#8b5cf6'
          }
        ]
      }
})

const claudeGuide = computed<ClientGuide>(() => {
  const baseUrl = apiBaseUrl.value || window.location.origin
  const terminalEnv = `export ANTHROPIC_BASE_URL="${baseUrl}"
export ANTHROPIC_AUTH_TOKEN="你的_API_Key"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`

  const settingsJson = `{
  "env": {
    "ANTHROPIC_BASE_URL": "${baseUrl}",
    "ANTHROPIC_AUTH_TOKEN": "你的_API_Key",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
    "CLAUDE_CODE_ATTRIBUTION_HEADER": "0"
  }
}`

  return isZh.value
    ? {
        id: 'claude',
        label: 'Claude Code 客户端',
        icon: 'cpu',
        eyebrow: 'Claude Code',
        title: 'Claude Code 客户端接入',
        summary: '这里不再写网页版思路，直接按客户端接入来配。推荐优先用环境变量验证，再决定是否写入本地配置文件长期保存。',
        steps: [
          { title: '准备 API Key', desc: '先在后台生成可用 Key，并确认该 Key 所属分组可以访问目标模型。' },
          { title: '设置环境变量', desc: '在终端写入 `ANTHROPIC_BASE_URL` 与 `ANTHROPIC_AUTH_TOKEN`，让 Claude Code 指向本平台。' },
          { title: '固化配置', desc: '如果希望每次打开都生效，再把相同配置写入 `~/.claude/settings.json`。' }
        ],
        snippets: [
          {
            title: 'Terminal / PowerShell',
            path: '当前终端会话',
            code: terminalEnv
          },
          {
            title: 'settings.json',
            path: '~/.claude/settings.json',
            code: settingsJson
          }
        ],
        tips: [
          {
            title: '先终端验证，再写文件',
            desc: '先在当前终端验证接通最省时间，确认无误后再保存成长期配置。',
            icon: 'terminal',
            color: '#f97316'
          },
          {
            title: '关闭非必要流量',
            desc: '保留 `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`，能减少无关流量干扰。',
            icon: 'shield',
            color: '#38bdf8'
          },
          {
            title: '一眼检查两项',
            desc: '接不通时先看 `BASE_URL` 是否对、`AUTH_TOKEN` 是否用的是平台生成的 Key。',
            icon: 'checkCircle',
            color: '#8b5cf6'
          }
        ]
      }
    : {
        id: 'claude',
        label: 'Claude Code Client',
        icon: 'cpu',
        eyebrow: 'Claude Code',
        title: 'Connect Claude Code',
        summary: 'This guide is client-first. Validate with environment variables first, then persist the same values in `~/.claude/settings.json` if needed.',
        steps: [
          { title: 'Prepare an API key', desc: 'Generate a valid key and make sure its group can reach the model you want.' },
          { title: 'Set environment variables', desc: 'Point Claude Code to this gateway with `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`.' },
          { title: 'Persist the config', desc: 'If you want it permanent, write the same values into `~/.claude/settings.json`.' }
        ],
        snippets: [
          { title: 'Terminal / PowerShell', path: 'Current terminal session', code: terminalEnv },
          { title: 'settings.json', path: '~/.claude/settings.json', code: settingsJson }
        ],
        tips: [
          {
            title: 'Verify in terminal first',
            desc: 'A temporary shell test is faster than editing permanent files before the first success.',
            icon: 'terminal',
            color: '#f97316'
          },
          {
            title: 'Keep nonessential traffic off',
            desc: 'Leaving `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` reduces background noise.',
            icon: 'shield',
            color: '#38bdf8'
          },
          {
            title: 'Check URL and token first',
            desc: 'Most setup failures come from a wrong base URL or using the wrong key.',
            icon: 'checkCircle',
            color: '#8b5cf6'
          }
        ]
      }
})

const guides = computed<Record<ClientId, ClientGuide>>(() => ({
  codex: codexGuide.value,
  claude: claudeGuide.value
}))

const clientTabs = computed(() => [guides.value.codex, guides.value.claude])
const currentGuide = computed(() => guides.value[activeClient.value])

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(isZh.value ? '代码已复制' : 'Copied')
  } catch {
    appStore.showError(isZh.value ? '复制失败，请手动复制' : 'Copy failed')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.tutorial-shell {
  background:
    radial-gradient(circle at 12% 14%, rgba(20, 184, 166, 0.14), transparent 24%),
    radial-gradient(circle at 84% 18%, rgba(59, 130, 246, 0.12), transparent 24%),
    linear-gradient(180deg, #f8fafc 0%, #edf5ff 46%, #f8fafc 100%);
}

.tutorial-noise {
  opacity: 0.24;
  background-image:
    radial-gradient(rgba(15, 23, 42, 0.08) 0.6px, transparent 0.6px),
    radial-gradient(rgba(15, 23, 42, 0.04) 0.6px, transparent 0.6px);
  background-position: 0 0, 18px 18px;
  background-size: 36px 36px;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.55), transparent 88%);
}

.tutorial-grid {
  opacity: 0.4;
  background-image:
    linear-gradient(rgba(148, 163, 184, 0.16) 1px, transparent 1px),
    linear-gradient(90deg, rgba(148, 163, 184, 0.16) 1px, transparent 1px);
  background-size: 72px 72px;
  mask-image: radial-gradient(circle at center, black 26%, transparent 84%);
}

.tutorial-spot {
  border-radius: 9999px;
  filter: blur(110px);
  opacity: 0.68;
}

.tutorial-spot-a {
  left: -8rem;
  top: 10rem;
  height: 16rem;
  width: 16rem;
  background: rgba(20, 184, 166, 0.18);
}

.tutorial-spot-b {
  right: -6rem;
  top: 14rem;
  height: 14rem;
  width: 14rem;
  background: rgba(59, 130, 246, 0.16);
}

.landing-display,
.brand-wordmark {
  font-family:
    'Avenir Next',
    'Segoe UI Variable Display',
    'PingFang SC',
    'Hiragino Sans GB',
    'Microsoft YaHei',
    sans-serif;
}

.tutorial-shell.is-dark .tutorial-noise {
  opacity: 0.1;
  background-image:
    radial-gradient(rgba(255, 255, 255, 0.12) 0.6px, transparent 0.6px),
    radial-gradient(rgba(255, 255, 255, 0.06) 0.6px, transparent 0.6px);
}

.tutorial-shell.is-dark .tutorial-grid {
  opacity: 0.18;
  background-image:
    linear-gradient(rgba(56, 189, 248, 0.12) 1px, transparent 1px),
    linear-gradient(90deg, rgba(56, 189, 248, 0.12) 1px, transparent 1px);
}

.site-logo-light {
  filter: none;
}

.site-logo-dark {
  filter: brightness(1.15) contrast(1.08) saturate(1.08);
}
</style>
