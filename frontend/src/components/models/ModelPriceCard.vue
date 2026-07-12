<template>
  <article
    class="premium-model-card"
    :style="platformStyle"
  >
    <div class="card-glow"></div>

    <header class="card-header">
      <div class="badge-row">
        <div class="brand-badge">
          <PlatformIcon :platform="model.platform as GroupPlatform" size="sm" />
          <span>{{ model.platform }}</span>
        </div>
        <div class="type-badge">{{ billingModeLabel }}</div>
      </div>

      <div class="title-row">
        <button
          type="button"
          class="model-title"
          :title="`${model.name} · ${t('modelSquare.copyModel')}`"
          @click="copyModelName"
        >
          <span>{{ model.name }}</span>
        </button>
        <button
          type="button"
          class="action-btn"
          :title="t('modelSquare.copyModel')"
          @click="copyModelName"
        >
          <Icon name="copy" size="sm" />
        </button>
      </div>
    </header>

    <div v-if="!pricing" class="no-pricing">
      <Icon name="inbox" size="lg" />
      <span>{{ t('modelSquare.noPricing') }}</span>
    </div>

    <div v-else class="metrics-grid">
      <div
        v-for="row in priceRows"
        :key="row.key"
        class="metric-box"
        :class="metricBoxClass(row.key)"
      >
        <div class="metric-header">
          <Icon :name="row.icon" size="sm" class="metric-icon" />
          <span class="metric-label">{{ row.label }}</span>
        </div>
        <div class="metric-value">
          <span
            class="price"
            :class="{ 'price--adjusted': showMultiplier }"
            :title="primaryPrice(row.value, row.scale)"
          >{{ primaryPrice(row.value, row.scale) }}</span>
          <span class="unit">{{ displayUnit(row) }}</span>
        </div>
        <div v-if="showMultiplier && row.value != null" class="adjusted-meta">
          <span class="base-price">{{ basePrice(row.value, row.scale) }} {{ displayUnit(row) }}</span>
          <span class="multiplier-pill">
            <Icon name="bolt" size="xs" class="multiplier-icon" />
            <span>×{{ multiplierLabel }}</span>
          </span>
        </div>
      </div>
    </div>

    <footer class="card-footer">
      <div class="footer-left">
        <span class="status-dot"></span>
        <span class="channel-name" :title="channelLabel">{{ channelLabel }}</span>
      </div>
      <div class="footer-actions">
        <button
          v-if="pricing && pricing.intervals.length > 0"
          type="button"
          class="tier-btn"
          :title="t('modelSquare.tierPricing', { count: pricing.intervals.length })"
          @click="showIntervals = true"
        >
          <Icon name="chart" size="xs" />
        </button>
        <div class="footer-right" title="充值比例：¥1 = $1">
          <Icon name="creditCard" size="xs" class="ratio-icon" />
          <span class="ratio-currency">¥1</span>
          <span class="ratio-equals">=</span>
          <span class="ratio-currency">$1</span>
        </div>
      </div>
    </footer>

    <Teleport to="body">
      <div
        v-if="showIntervals && pricing"
        class="fixed inset-0 z-[100000] flex items-center justify-center bg-black/45 p-4"
        role="dialog"
        aria-modal="true"
        :aria-label="t('modelSquare.tierDialogTitle', { model: model.name })"
        @click.self="showIntervals = false"
      >
        <div class="max-h-[min(80vh,680px)] w-full max-w-2xl overflow-hidden rounded-lg bg-white shadow-2xl dark:bg-dark-900">
          <div class="flex items-center justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
            <div class="min-w-0">
              <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ model.name }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ t('modelSquare.tierPricingTitle') }}</p>
            </div>
            <button
              type="button"
              class="btn-icon flex-shrink-0"
              :title="t('common.close')"
              @click="showIntervals = false"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <div class="overflow-auto p-5">
            <table class="w-full min-w-[560px] text-left text-xs">
              <thead class="text-gray-500 dark:text-gray-400">
                <tr class="border-b border-gray-200 dark:border-dark-700">
                  <th class="px-2 py-2 font-medium">{{ t('modelSquare.interval') }}</th>
                  <th class="px-2 py-2 font-medium">{{ t('modelSquare.input') }}</th>
                  <th class="px-2 py-2 font-medium">{{ t('modelSquare.output') }}</th>
                  <th class="px-2 py-2 font-medium">{{ t('modelSquare.cacheWrite') }}</th>
                  <th class="px-2 py-2 font-medium">{{ t('modelSquare.cacheRead') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="(interval, index) in pricing.intervals"
                  :key="index"
                  class="border-b border-gray-100 last:border-0 dark:border-dark-800"
                >
                  <td class="whitespace-nowrap px-2 py-3 text-gray-700 dark:text-gray-300">
                    {{ interval.tier_label || intervalRange(interval.min_tokens, interval.max_tokens) }}
                  </td>
                  <td class="px-2 py-3 font-semibold tabular-nums" :class="priceTextClass">{{ primaryPrice(interval.input_price, 1_000_000) }}</td>
                  <td class="px-2 py-3 font-semibold tabular-nums" :class="priceTextClass">{{ primaryPrice(interval.output_price, 1_000_000) }}</td>
                  <td class="px-2 py-3 font-semibold tabular-nums" :class="priceTextClass">{{ primaryPrice(interval.cache_write_price, 1_000_000) }}</td>
                  <td class="px-2 py-3 font-semibold tabular-nums" :class="priceTextClass">{{ primaryPrice(interval.cache_read_price, 1_000_000) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </Teleport>
  </article>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import modelSquareMessages from '@/i18n/modelSquare'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { platformTextClass } from '@/utils/platformColors'
import { formatModelPrice, type EffectiveMultiplier, type ModelPricingVariant, type ModelSquareModel } from '@/utils/modelSquare'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
} from '@/constants/channel'
import type { GroupPlatform } from '@/types'

const props = defineProps<{
  model: ModelSquareModel
  variant: ModelPricingVariant | null
  showMultiplier: boolean
  multiplier: EffectiveMultiplier | null
  pricePlatform?: string
}>()

const { t } = useI18n({ useScope: 'local', messages: modelSquareMessages })
const { copyToClipboard } = useClipboard()
const showIntervals = ref(false)
const pricing = computed(() => props.variant?.pricing ?? null)

const billingModeLabel = computed(() => {
  switch (pricing.value?.billing_mode) {
    case BILLING_MODE_IMAGE:
      return t('modelSquare.billingImage')
    case BILLING_MODE_PER_REQUEST:
      return t('modelSquare.billingRequest')
    case BILLING_MODE_TOKEN:
      return t('modelSquare.billingToken')
    default:
      return t('modelSquare.billingUnknown')
  }
})

const channelLabel = computed(() => {
  const names = props.variant ? [props.variant.channelName] : props.model.channelNames
  if (names.length === 0) return t('modelSquare.channelUnknown')
  if (names.length === 1) return names[0]
  return t('modelSquare.channelCount', { count: names.length })
})

type PriceRow = {
  key: string
  label: string
  value: number | null
  scale: number
  unit: string
  icon: 'upload' | 'download' | 'database' | 'sparkles' | 'bolt'
  toneClass: string
}

const priceRows = computed<PriceRow[]>(() => {
  const value = pricing.value
  if (!value) return []

  if (value.billing_mode === BILLING_MODE_IMAGE) {
    return [
      {
        key: 'image',
        label: t('modelSquare.imageOutput'),
        value: value.image_output_price ?? value.per_request_price,
        scale: 1,
        unit: t('modelSquare.perRequest'),
        icon: 'sparkles',
        toneClass: 'text-pink-500 dark:text-pink-400',
      },
    ]
  }

  if (value.billing_mode === BILLING_MODE_PER_REQUEST) {
    return [
      {
        key: 'request',
        label: t('modelSquare.requestPrice'),
        value: value.per_request_price,
        scale: 1,
        unit: t('modelSquare.perRequest'),
        icon: 'bolt',
        toneClass: 'text-amber-500 dark:text-amber-400',
      },
    ]
  }

  const rows: PriceRow[] = [
    { key: 'input', label: t('modelSquare.input'), value: value.input_price, scale: 1_000_000, unit: t('modelSquare.perMillion'), icon: 'upload' as const, toneClass: 'text-emerald-500 dark:text-emerald-400' },
    { key: 'output', label: t('modelSquare.output'), value: value.output_price, scale: 1_000_000, unit: t('modelSquare.perMillion'), icon: 'download' as const, toneClass: 'text-blue-500 dark:text-blue-400' },
    { key: 'cacheWrite', label: t('modelSquare.cacheWrite'), value: value.cache_write_price, scale: 1_000_000, unit: t('modelSquare.perMillion'), icon: 'database' as const, toneClass: 'text-amber-500 dark:text-amber-400' },
    { key: 'cacheRead', label: t('modelSquare.cacheRead'), value: value.cache_read_price, scale: 1_000_000, unit: t('modelSquare.perMillion'), icon: 'database' as const, toneClass: 'text-violet-500 dark:text-violet-400' },
  ]
  if (value.image_output_price != null && value.image_output_price > 0) {
    rows.push({ key: 'imageOutput', label: t('modelSquare.imageOutput'), value: value.image_output_price, scale: 1_000_000, unit: t('modelSquare.perMillion'), icon: 'sparkles', toneClass: 'text-pink-500 dark:text-pink-400' })
  }
  return rows
})

const effectivePricePlatform = computed(() => props.pricePlatform || props.model.platform)

const priceTextClass = computed(() => {
  if (!props.showMultiplier) return 'text-gray-900 dark:text-white'
  return platformTextClass(effectivePricePlatform.value)
})

const multiplierLabel = computed(() => {
  const value = props.multiplier?.value ?? 1
  return `${Number(value.toPrecision(8))}`
})

function primaryPrice(value: number | null, scale: number): string {
  return formatModelPrice(value, {
    scale,
    multiplier: props.showMultiplier ? (props.multiplier?.value ?? 1) : 1,
    symbol: '$',
  })
}

type PlatformPalette = {
  color: string
  background: string
  glow: string
}

const PLATFORM_PALETTES: Record<string, PlatformPalette> = {
  anthropic: {
    color: '#ea580c',
    background: '#fff7ed',
    glow: 'rgba(234, 88, 12, 0.08)',
  },
  openai: {
    color: '#16a34a',
    background: '#f0fdf4',
    glow: 'rgba(22, 163, 74, 0.08)',
  },
  gemini: {
    color: '#2563eb',
    background: '#eff6ff',
    glow: 'rgba(37, 99, 235, 0.08)',
  },
  antigravity: {
    color: '#9333ea',
    background: '#faf5ff',
    glow: 'rgba(147, 51, 234, 0.08)',
  },
  grok: {
    color: '#27272a',
    background: '#e8eaed',
    glow: 'rgba(39, 39, 42, 0.07)',
  },
}

function paletteFor(platform: string): PlatformPalette {
  return PLATFORM_PALETTES[platform.toLowerCase()] ?? {
    color: '#0d9488',
    background: '#f0fdfa',
    glow: 'rgba(13, 148, 136, 0.08)',
  }
}

const platformStyle = computed(() => {
  const brand = paletteFor(props.model.platform)
  const priceBrand = paletteFor(effectivePricePlatform.value)
  return {
    '--brand-color': brand.color,
    '--brand-bg': brand.background,
    '--brand-glow': brand.glow,
    '--price-brand': priceBrand.color,
    '--price-brand-bg': priceBrand.background,
  }
})

function metricBoxClass(key: string): string {
  if (key === 'input' || key === 'request') return 'box-input'
  if (key === 'output' || key === 'image') return 'box-output'
  if (key === 'cacheWrite') return 'box-cache-write'
  if (key === 'cacheRead') return 'box-cache-read'
  return 'box-image'
}

function displayUnit(row: PriceRow): string {
  return row.scale === 1_000_000 ? '/ 1M' : row.unit
}

function basePrice(value: number | null, scale: number): string {
  return formatModelPrice(value, { scale, symbol: '$' })
}

function intervalRange(min: number, max: number | null): string {
  return max == null ? `>${min}` : `${min}–${max}`
}

async function copyModelName() {
  await copyToClipboard(props.model.name, t('modelSquare.modelCopied'))
}

function handleEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') showIntervals.value = false
}

onMounted(() => window.addEventListener('keydown', handleEscape))
onBeforeUnmount(() => window.removeEventListener('keydown', handleEscape))
</script>

<style scoped>
.premium-model-card {
  position: relative;
  display: flex;
  width: 100%;
  min-width: 0;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid #eaeaea;
  border-radius: 20px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.02), 0 8px 24px rgba(0, 0, 0, 0.04);
  font-family: Inter, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  transition: transform 0.3s cubic-bezier(0.16, 1, 0.3, 1), box-shadow 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.premium-model-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.03), 0 16px 40px rgba(0, 0, 0, 0.08);
}

.card-glow {
  position: absolute;
  z-index: 0;
  top: -50px;
  left: -50px;
  width: 200px;
  height: 200px;
  background: radial-gradient(circle, var(--brand-glow) 0%, transparent 70%);
  pointer-events: none;
}

.card-header {
  position: relative;
  z-index: 1;
  padding: 20px 20px 15px;
  border-bottom: 1px solid #f0f0f0;
}

.badge-row,
.title-row,
.metric-header,
.metric-value,
.card-footer,
.footer-left,
.footer-actions,
.footer-right {
  display: flex;
  align-items: center;
}

.badge-row,
.card-footer {
  justify-content: space-between;
}

.badge-row {
  margin-bottom: 14px;
}

.brand-badge,
.type-badge {
  border-radius: 999px;
  font-size: 12px;
  line-height: 1;
}

.brand-badge {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  background: var(--brand-bg);
  color: var(--brand-color);
  font-weight: 600;
  text-transform: uppercase;
}

.type-badge {
  flex-shrink: 0;
  padding: 7px 10px;
  background: #f5f5f5;
  color: #737373;
  font-weight: 500;
}

.title-row {
  min-width: 0;
  gap: 8px;
}

.model-title {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  color: #0a0a0a;
  font-size: 18px;
  font-weight: 700;
  line-height: 1.35;
  text-align: left;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.model-title span {
  overflow: hidden;
  text-overflow: ellipsis;
}

.action-btn,
.tier-btn {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  border: 1px solid transparent;
  background: transparent;
  color: #a3a3a3;
  transition: all 0.2s ease;
}

.action-btn {
  width: 28px;
  height: 28px;
  border-radius: 8px;
}

.action-btn:hover,
.tier-btn:hover {
  border-color: #e4e4e7;
  background: #f4f4f5;
  color: #0a0a0a;
}

.metrics-grid {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  padding: 16px 20px 20px;
}

.metric-box {
  display: flex;
  min-width: 0;
  height: 96px;
  min-height: 96px;
  box-sizing: border-box;
  flex-direction: column;
  justify-content: flex-start;
  padding: 11px 13px;
  border: 1px solid #f0f0f0;
  border-radius: 14px;
  background: #fafafa;
  transition: background-color 0.25s ease, border-color 0.25s ease;
}

.metric-box:only-child {
  grid-column: 1 / -1;
}

.metric-box:last-child:nth-child(odd) {
  grid-column: 1 / -1;
}

.metric-header {
  min-width: 0;
  gap: 8px;
}

.metric-icon {
  flex-shrink: 0;
  color: #737373;
  transition: color 0.25s ease;
}

.metric-label {
  overflow: hidden;
  color: #737373;
  font-size: 12px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-value {
  min-width: 0;
  align-items: baseline;
  margin-top: auto;
  gap: 4px;
}

.price {
  overflow: hidden;
  color: #0a0a0a;
  font-family: "JetBrains Mono", ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 18px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.price.price--adjusted {
  color: var(--price-brand);
}

.unit {
  overflow: hidden;
  color: #a3a3a3;
  font-size: 11px;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.adjusted-meta {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  min-height: 14px;
  margin-top: 4px;
  color: #a3a3a3;
  font-size: 10px;
  font-weight: 500;
  line-height: 1;
}
.base-price {
  overflow: hidden;
  color: #a3a3a3;
  font-variant-numeric: tabular-nums;
  text-decoration-line: line-through;
  text-decoration-color: #a3a3a3;
  text-decoration-thickness: 1px;
  text-overflow: ellipsis;
  white-space: nowrap;
}


.multiplier-pill {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  gap: 2px;
  padding: 2px 6px;
  border-radius: 999px;
  background: var(--price-brand-bg);
  color: var(--price-brand);
  font-weight: 700;
}


.multiplier-icon {
  width: 10px;
  height: 10px;
  stroke-width: 2;
}
.box-input:hover { border-color: #a7f3d0; background: #ecfdf5; }
.box-input:hover .metric-icon { color: #10b981; }
.box-output:hover { border-color: #bfdbfe; background: #eff6ff; }
.box-output:hover .metric-icon { color: #3b82f6; }
.box-cache-write:hover { border-color: #fde68a; background: #fffbeb; }
.box-cache-write:hover .metric-icon { color: #f59e0b; }
.box-cache-read:hover { border-color: #ddd6fe; background: #f5f3ff; }
.box-cache-read:hover .metric-icon { color: #8b5cf6; }
.box-image:hover { border-color: #fbcfe8; background: #fdf2f8; }
.box-image:hover .metric-icon { color: #ec4899; }

.no-pricing {
  position: relative;
  z-index: 1;
  display: flex;
  min-height: 228px;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: #a3a3a3;
  font-size: 13px;
  font-weight: 500;
}

.card-footer {
  position: relative;
  z-index: 1;
  margin-top: auto;
  min-width: 0;
  gap: 10px;
  padding: 13px 20px;
  border-top: 1px solid #eaeaea;
  background: #fbfbfb;
}

.footer-left {
  min-width: 0;
  gap: 8px;
  color: #737373;
  font-size: 12px;
  font-weight: 500;
}

.status-dot {
  width: 7px;
  height: 7px;
  flex-shrink: 0;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.18);
}

.channel-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.footer-actions,
.footer-right {
  flex-shrink: 0;
}

.footer-actions {
  gap: 6px;
}

.tier-btn {
  width: 26px;
  height: 26px;
  border-radius: 7px;
}

.footer-right {
  gap: 5px;
  min-height: 26px;
  padding: 5px 9px;
  border: 1px solid color-mix(in srgb, var(--brand-color) 20%, #e5e7eb);
  border-radius: 8px;
  background: color-mix(in srgb, var(--brand-bg) 76%, #fff);
  color: var(--brand-color);
  font-family: "JetBrains Mono", ui-monospace, SFMono-Regular, Consolas, monospace;
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.025);
}

.ratio-icon {
  margin-right: 1px;
  opacity: 0.72;
}

.ratio-currency {
  font-variant-numeric: tabular-nums;
}

.ratio-equals {
  color: #a3a3a3;
  font-weight: 500;
}

:global(.dark) .premium-model-card {
  border-color: #334155;
  background: #0f172a;
}

:global(.dark) .type-badge,
:global(.dark) .metric-box {
  background: #1e293b;
}

:global(.dark) .model-title {
  color: #f8fafc;
}

:global(.dark) .type-badge,
:global(.dark) .metric-label,
:global(.dark) .metric-icon,
:global(.dark) .footer-left {
  color: #cbd5e1;
}

:global(.dark) .metric-box {
  border-color: #334155;
}

:global(.dark) .price {
  color: #f8fafc;
}

:global(.dark) .price.price--adjusted {
  color: var(--price-brand);
}

:global(.dark) .card-footer {
  border-color: #334155;
  background: rgba(30, 41, 59, 0.65);
}

:global(.dark) .card-header {
  border-color: #334155;
}

:global(.dark) .footer-right {
  border-color: color-mix(in srgb, var(--brand-color) 34%, #334155);
  background: color-mix(in srgb, var(--brand-color) 12%, #0f172a);
}

:global(.dark) .box-input:hover { border-color: rgba(16, 185, 129, 0.45); background: rgba(16, 185, 129, 0.1); }
:global(.dark) .box-output:hover { border-color: rgba(59, 130, 246, 0.45); background: rgba(59, 130, 246, 0.1); }
:global(.dark) .box-cache-write:hover { border-color: rgba(245, 158, 11, 0.45); background: rgba(245, 158, 11, 0.1); }
:global(.dark) .box-cache-read:hover { border-color: rgba(139, 92, 246, 0.45); background: rgba(139, 92, 246, 0.1); }
:global(.dark) .box-image:hover { border-color: rgba(236, 72, 153, 0.45); background: rgba(236, 72, 153, 0.1); }

@media (max-width: 640px) {
  .card-header {
    padding: 18px 18px 15px;
  }

  .metrics-grid {
    gap: 10px;
    padding: 14px 18px 18px;
  }

  .metric-box {
    padding: 12px;
  }

  .card-footer {
    padding: 12px 18px;
  }
}
</style>
