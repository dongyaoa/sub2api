<template>
  <div class="space-y-5">
    <div v-if="filteredAmounts.length > 0">
      <label class="mb-2 block text-sm font-semibold text-gray-800 dark:text-gray-200">
        {{ t('payment.quickAmounts') }}
      </label>
      <div class="grid grid-cols-3 gap-2 sm:grid-cols-4 xl:grid-cols-5">
        <button
          v-for="amt in filteredAmounts"
          :key="amt"
          type="button"
          :class="[
            'group relative flex min-h-[58px] items-center justify-center overflow-hidden rounded-xl border px-2.5 py-2 text-center',
            modelValue === amt
              ? 'border-amber-300 bg-amber-50/30 text-gray-950 shadow-md shadow-amber-500/10 ring-1 ring-amber-200/70 dark:border-amber-300/70 dark:bg-amber-950/15 dark:text-white'
              : 'border-gray-200 bg-white text-gray-800 shadow-sm dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100',
          ]"
          @click="selectAmount(amt)"
        >
          <span class="inline-flex items-baseline justify-center leading-none">
            <span
              :class="[
                'mr-1 text-xs font-black sm:text-sm',
                modelValue === amt ? 'text-primary-600 dark:text-primary-200' : 'text-primary-500 dark:text-primary-300',
              ]"
            >{{ amountSymbol }}</span>
            <span class="text-base font-black tracking-tight sm:text-lg">{{ formatQuickAmountNumber(amt) }}</span>
          </span>
        </button>
      </div>
    </div>

    <div>
      <label class="mb-2 block text-sm font-semibold text-gray-800 dark:text-gray-200">
        {{ t('payment.customAmount') }}
      </label>
      <div class="relative rounded-xl border border-gray-200 bg-white shadow-sm transition-all focus-within:border-primary-500 focus-within:shadow-lg focus-within:shadow-primary-500/10 focus-within:ring-2 focus-within:ring-primary-500/15 dark:border-dark-600 dark:bg-dark-800">
        <span class="absolute left-4 top-1/2 inline-flex -translate-y-1/2 items-baseline gap-2 text-xs font-bold uppercase tracking-wide text-gray-400 dark:text-dark-400">
          <span class="text-base font-black text-primary-500 dark:text-primary-300">{{ amountSymbol }}</span>
          <span>{{ normalizedCurrency }}</span>
        </span>
        <input
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          class="w-full bg-transparent py-3.5 pl-24 pr-4 text-lg font-semibold text-gray-950 outline-none placeholder:text-sm placeholder:font-normal placeholder:text-gray-400 dark:text-white dark:placeholder:text-dark-400"
          @input="handleInput"
        />
      </div>
      <p class="mt-3 rounded-xl border border-primary-100 bg-primary-50/70 px-3 py-2 text-xs font-medium leading-relaxed text-primary-700 dark:border-primary-900/40 dark:bg-primary-950/30 dark:text-primary-200">
        支付实时到账，如果遇到支付后未到账可联系客服提供订单号！
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { currencySymbol, formatPaymentAmount, normalizePaymentCurrency } from './currency'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  currency?: string
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  currency: undefined,
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')
const normalizedCurrency = computed(() => normalizePaymentCurrency(props.currency))
const amountSymbol = computed(() => currencySymbol(normalizedCurrency.value))

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${formatAmount(props.min)} - ${formatAmount(props.max)}`
  if (props.min > 0) return `>= ${formatAmount(props.min)}`
  if (props.max > 0) return `<= ${formatAmount(props.max)}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function formatAmount(value: number): string {
  return formatPaymentAmount(value, normalizedCurrency.value)
}

function formatQuickAmountNumber(value: number): string {
  return new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 0,
  }).format(Number.isFinite(value) ? Math.round(value) : 0)
}


function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const input = e.target as HTMLInputElement
  const val = input.value
  if (!AMOUNT_PATTERN.test(val)) {
    input.value = customText.value
    return
  }
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (!isNaN(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>