<template>
  <section class="promotion-editor min-w-0 rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" :aria-label="ta('heading')">
    <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 px-4 py-4 dark:border-dark-700 sm:px-6">
      <div class="min-w-0">
        <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ ta('heading') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ ta('description') }}</p>
      </div>
      <span v-if="hasUnsavedChanges" class="rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400" data-testid="promotion-unsaved">{{ ta('unsaved') }}</span>
    </div>

    <div v-if="loading" class="p-6 text-sm text-gray-500 dark:text-gray-400" role="status">{{ ta('loading') }}</div>
    <div v-else-if="!loaded" class="flex flex-wrap items-center gap-3 p-6">
      <p role="alert" class="text-sm text-red-600 dark:text-red-300">{{ error }}</p>
      <button type="button" class="btn btn-secondary" @click="load">{{ ta('retry') }}</button>
    </div>

    <div v-else class="settings-content p-4 sm:p-6">
      <p v-if="!savedConfig" class="settings-wide text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ ta('sample') }}</p>
      <fieldset :disabled="saving || loading" class="min-w-0 space-y-6">
        <div class="space-y-4">
          <div class="flex items-center justify-between gap-4">
            <div class="min-w-0">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ ta('basics') }}</h3>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ ta('enabledHint') }}</p>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <span class="text-xs text-gray-500 dark:text-gray-400">{{ ta('enabled') }}</span>
              <Toggle v-model="draft.enabled" :aria-label="ta('enabled')" :disabled="saving || loading" data-testid="promotion-enabled" />
            </div>
          </div>
          <div class="grid min-w-0 gap-4 lg:grid-cols-2">
            <label class="block min-w-0 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ ta('title') }}
              <input v-model="draft.title" class="input mt-1.5 w-full min-w-0" type="text" maxlength="80" data-testid="promotion-title" />
            </label>
            <label class="block min-w-0 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ ta('subtitle') }}
              <input v-model="draft.subtitle" class="input mt-1.5 w-full min-w-0" type="text" maxlength="200" data-testid="promotion-subtitle" />
            </label>
          </div>
        </div>

        <div class="min-w-0 space-y-4 border-t border-gray-100 pt-5 dark:border-dark-700">
          <div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ ta('schedule') }}</h3>
            <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ ta('timezone') }}</p>
          </div>
          <div class="grid min-w-0 gap-4 lg:grid-cols-2">
            <label class="block min-w-0 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ ta('starts') }}
              <input v-model="startsAt" class="input mt-1.5 w-full min-w-0 max-w-full" type="datetime-local" data-testid="promotion-starts" />
            </label>
            <label class="block min-w-0 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ ta('ends') }}
              <input v-model="endsAt" class="input mt-1.5 w-full min-w-0 max-w-full" type="datetime-local" data-testid="promotion-ends" />
            </label>
          </div>
          <label class="block min-w-0 text-sm font-medium text-gray-700 dark:text-gray-300 lg:max-w-xs">
            {{ ta('currency') }}
            <select v-model="draft.currency" class="input mt-1.5 w-full min-w-0" data-testid="promotion-currency">
              <option v-for="currency in currencies" :key="currency" :value="currency">{{ currency }}</option>
            </select>
          </label>
          <p class="text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ ta('currencyHint') }}</p>
        </div>

        <div class="min-w-0 border-t border-gray-100 pt-5 dark:border-dark-700">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ ta('tiers') }}</h3>
            <button type="button" class="btn btn-secondary px-3 py-1.5 text-xs" :disabled="saving || loading || draft.tiers.length >= 10" data-testid="promotion-add-tier" @click="addTier">+ {{ ta('addTier') }}</button>
          </div>
          <div class="space-y-3">
            <div v-for="(tier, index) in draft.tiers" :key="index" class="grid min-w-0 grid-cols-[minmax(0,1fr)_minmax(0,1fr)_2rem] items-end gap-2 sm:gap-3" data-testid="promotion-tier">
              <label class="block min-w-0 text-xs font-medium text-gray-600 dark:text-gray-300">
                {{ ta('threshold') }} ({{ draft.currency }})
                <input v-model.number="tier.min_amount" class="input mt-1.5 w-full min-w-0 px-2 sm:px-3" type="number" min="0.01" max="1000000000" step="0.01" :data-testid="`promotion-threshold-${index}`" />
              </label>
              <label class="block min-w-0 text-xs font-medium text-gray-600 dark:text-gray-300">
                {{ ta('percent') }} (%)
                <input v-model.number="tier.bonus_percent" class="input mt-1.5 w-full min-w-0 px-2 sm:px-3" type="number" min="0.01" max="100" step="0.01" :data-testid="`promotion-percent-${index}`" />
              </label>
              <button type="button" class="mb-1 flex h-8 w-8 items-center justify-center rounded-md text-lg text-gray-400 hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-700 dark:hover:text-gray-200" :disabled="saving || loading || draft.tiers.length <= 1" :aria-label="`${ta('removeTier')} ${index + 1}`" @click="draft.tiers.splice(index, 1)"><span aria-hidden="true">×</span></button>
            </div>
          </div>
          <p class="mt-3 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ ta('tiersHint') }}</p>
        </div>

        <div class="min-w-0 border-t border-gray-100 pt-5 dark:border-dark-700">
          <label class="block min-w-0 text-sm font-medium text-gray-700 dark:text-gray-300 lg:max-w-xs">
            {{ ta('cap') }}
            <input v-model.number="draft.max_bonus" class="input mt-1.5 w-full min-w-0" type="number" min="0" max="1000000000" step="0.01" data-testid="promotion-cap" />
          </label>
          <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ ta('capHint') }}</p>
        </div>
      </fieldset>

      <aside class="settings-preview">
        <div class="preview-offer">
          <div class="flex items-center gap-2 text-xs font-medium text-amber-800 dark:text-amber-200"><Icon name="sparkles" size="sm" />{{ ta('preview') }}</div>
          <h3 class="mt-4 break-words text-lg font-semibold tracking-tight text-gray-900 dark:text-white">{{ draft.title || ta('heading') }}</h3>
          <p v-if="draft.subtitle" class="mt-2 break-words text-xs leading-5 text-gray-500 dark:text-gray-400">{{ draft.subtitle }}</p>
          <div class="mt-4 space-y-2"><div v-for="(tier, index) in sortedTiers" :key="index" class="preview-tier"><span>{{ formatPaymentAmount(tier.min_amount, draft.currency, locale) }}</span><strong>+{{ tier.bonus_percent }}%</strong></div></div>
          <p class="mt-4 text-[11px] leading-5 text-gray-500 dark:text-gray-400">{{ ta('previewRule') }}</p>
        </div>
        <div class="preview-calculator">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ ta('calculator') }}</h3>
          <label class="mt-4 block text-xs text-gray-500 dark:text-gray-400">{{ ta('testAmount') }} ({{ draft.currency }})<input v-model.number="previewAmount" type="number" min="0" max="1000000000" step="0.01" class="input mt-2 w-full min-w-0" data-testid="promotion-preview-amount" /></label>
          <dl class="mt-4 space-y-3 text-xs"><div class="flex justify-between gap-3"><dt class="text-gray-500 dark:text-gray-400">{{ ta('baseCredit') }}</dt><dd class="font-medium tabular-nums text-gray-900 dark:text-white">${{ amountPreview.baseAmount.toFixed(2) }}</dd></div><div class="flex justify-between gap-3"><dt class="text-gray-500 dark:text-gray-400">{{ ta('bonusCredit') }}</dt><dd class="font-semibold tabular-nums text-amber-700 dark:text-amber-300">+${{ amountPreview.bonusAmount.toFixed(2) }}</dd></div><div class="flex items-center justify-between gap-3 border-t border-gray-100 pt-3 dark:border-dark-700"><dt class="text-gray-600 dark:text-gray-300">{{ ta('totalCredit') }}</dt><dd class="text-lg font-semibold tabular-nums text-gray-900 dark:text-white">${{ amountPreview.totalAmount.toFixed(2) }}</dd></div></dl>
          <p class="mt-4 text-[11px] leading-5 text-gray-400">{{ ta('calculatorHint') }}</p>
        </div>
      </aside>
      <p v-if="error" role="alert" class="settings-wide text-sm text-red-600 dark:text-red-300">{{ error }}</p>
      <div class="settings-wide flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-5 dark:border-dark-700">
        <p v-if="success" role="status" class="text-sm text-primary-600 dark:text-primary-300">{{ ta('saved') }}</p>
        <p v-else class="text-xs text-gray-500 dark:text-gray-400">{{ ta(hasUnsavedChanges ? 'unsavedHint' : 'savedHint') }}</p>
        <button type="button" class="editor-save ml-auto" :disabled="saving || loading || !hasUnsavedChanges" data-testid="promotion-save" @click="save">{{ ta(saving ? 'saving' : 'save') }}</button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { calculateRechargePromotion } from '@/utils/rechargePromotion'
import { useI18n } from 'vue-i18n'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { RechargePromotion } from '@/types/payment'
import Toggle from '@/components/common/Toggle.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatPaymentAmount } from '@/components/payment/currency'
import { rechargePromotionMessages } from './rechargePromotionMessages'
import { defaultRechargePromotion, fromBeijingInput, promotionValidationError, toBeijingInput, writablePromotion } from './rechargePromotionSettings'

const emit = defineEmits<{ saved: [] }>()
const { locale } = useI18n()
function ta(key: string, params: Record<string, string | number> = {}): string {
  const messages: Record<string, string> = locale.value.startsWith('zh') ? rechargePromotionMessages.zh : rechargePromotionMessages.en
  return (messages[key] ?? key).replace(/\{(\w+)\}/g, (match, name: string) => String(params[name] ?? match))
}
const draft = ref(defaultRechargePromotion())
const savedConfig = ref<RechargePromotion | null>(null)
const startsAt = ref(toBeijingInput(draft.value.starts_at))
const endsAt = ref(toBeijingInput(draft.value.ends_at))
const loading = ref(true)
const loaded = ref(false)
const saving = ref(false)
const error = ref('')
const success = ref(false)
const savedDraft = ref('')
const draftFingerprint = computed(() => JSON.stringify({ ...draft.value, startsAt: startsAt.value, endsAt: endsAt.value }))
const hasUnsavedChanges = computed(() => loaded.value && draftFingerprint.value !== savedDraft.value)
const currencies = computed(() => [...new Set(['CNY', 'USD', draft.value.currency])])
const sortedTiers = computed(() => [...draft.value.tiers].sort((a, b) => a.min_amount - b.min_amount))
const previewAmount = ref(100)
const rechargeMultiplier = ref(1)
const amountPreview = computed(() => {
  const start = fromBeijingInput(startsAt.value)
  const end = fromBeijingInput(endsAt.value)
  const config = { ...draft.value, enabled: true, active: true, starts_at: start, ends_at: end }
  return calculateRechargePromotion(promotionValidationError(config) ? null : config, previewAmount.value, rechargeMultiplier.value, draft.value.currency, Date.parse(start))
})
watch(draftFingerprint, () => { if (hasUnsavedChanges.value) success.value = false })

async function load() {
  if (saving.value) return
  loading.value = true
  error.value = ''
  success.value = false
  try {
    const { data } = await adminPaymentAPI.getConfig()
    rechargeMultiplier.value = data.balance_recharge_multiplier || 1
    const config = data.recharge_promotion
    savedConfig.value = config && (config.enabled || config.title || config.tiers.length) ? config : null
    draft.value = savedConfig.value ? writablePromotion(savedConfig.value) : defaultRechargePromotion()
    if (!savedConfig.value && !locale.value.startsWith('zh')) draft.value.title = 'Recharge bonus'
    startsAt.value = toBeijingInput(draft.value.starts_at)
    endsAt.value = toBeijingInput(draft.value.ends_at)
    savedDraft.value = savedConfig.value ? draftFingerprint.value : ''
    loaded.value = true
  } catch (err) {
    error.value = extractApiErrorMessage(err, ta('loadFailed'))
  } finally {
    loading.value = false
  }
}

function addTier() {
  if (saving.value || loading.value || draft.value.tiers.length >= 10) return
  draft.value.tiers.push({ min_amount: Math.max(0, ...draft.value.tiers.map(tier => Number(tier.min_amount) || 0)) + 100, bonus_percent: 10 })
}

async function save() {
  if (saving.value || loading.value || !loaded.value || !hasUnsavedChanges.value) return
  error.value = ''
  success.value = false
  const payload = writablePromotion({ ...draft.value, starts_at: fromBeijingInput(startsAt.value), ends_at: fromBeijingInput(endsAt.value) })
  const validation = promotionValidationError(payload)
  if (validation) { error.value = ta(validation); return }
  saving.value = true
  try {
    await adminPaymentAPI.updateConfig({ recharge_promotion: payload })
    savedConfig.value = { ...payload, active: payload.enabled && Date.now() >= Date.parse(payload.starts_at) && Date.now() < Date.parse(payload.ends_at) }
    draft.value = writablePromotion(payload)
    startsAt.value = toBeijingInput(payload.starts_at)
    endsAt.value = toBeijingInput(payload.ends_at)
    savedDraft.value = draftFingerprint.value
    success.value = true
    emit('saved')
  } catch (err) {
    error.value = extractApiErrorMessage(err, ta('saveFailed'))
  } finally {
    saving.value = false
  }
}

defineExpose({ reload: load })
onMounted(() => { void load() })
</script>

<style scoped>
.settings-content { display: grid; gap: 24px; align-items: start; }
.settings-wide { grid-column: 1 / -1; }
.settings-preview { min-width: 0; display: grid; gap: 16px; }
.preview-offer { padding: 20px; border: 1px solid #e7d8b8; border-radius: 14px; background: linear-gradient(135deg,#fffdf7,#f8edd3); }
.preview-tier { display: flex; justify-content: space-between; gap: 12px; padding: 9px 11px; border: 1px solid #eadfc9; border-radius: 8px; background: #fffcf4b3; font-size: 12px; color: #8c754c; }
.preview-tier strong { color: #956018; font-weight: 600; }
.preview-calculator { padding: 20px; border: 1px solid #e5e7eb; border-radius: 14px; background: #fafbfc; }
.editor-save { border: 1px solid #ac7c28; border-radius: 9px; padding: 10px 20px; background: linear-gradient(110deg,#bf9447,#9b6e24); color: white; font-size: 13px; font-weight: 600; box-shadow: 0 2px 5px #9b6e241a; }
.editor-save:hover:not(:disabled) { filter: brightness(1.06); }
.editor-save:disabled { opacity: .45; cursor: not-allowed; }
.editor-save:focus-visible { outline: 2px solid #d1ab62; outline-offset: 3px; }
.dark .preview-offer { border-color: #5a4930; background: linear-gradient(135deg,#29292b,#3c3121); }
.dark .preview-tier { border-color: #584a32; background: #2c281faa; color: #c3b392; }
.dark .preview-tier strong { color: #eccb86; }
.dark .preview-calculator { border-color: #303847; background: #1b222f; }
.dark .editor-save { color: #251c0d; border-color: #cba354; background: linear-gradient(100deg,#ebd29b,#bf944b); }
@media (min-width: 1280px) {
  .settings-content { grid-template-columns: minmax(0,1fr) 300px; gap: 24px 32px; }
  .settings-preview { position: sticky; top: 24px; }
}
</style>
