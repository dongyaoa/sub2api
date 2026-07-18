<template>
  <div>
    <label v-if="props.showLabel" class="mb-3 block text-sm font-semibold text-gray-800 dark:text-gray-200">
      {{ t('payment.paymentMethod') }}
    </label>
    <div :class="gridClass">
      <button
        v-for="method in sortedMethods"
        :key="method.type"
        type="button"
        :disabled="!method.available"
        :class="[
          baseButtonClass,
          !method.available
            ? 'cursor-not-allowed border-gray-200 bg-gray-50 opacity-55 dark:border-dark-700 dark:bg-dark-800/50'
            : selected === method.type
              ? methodSelectedClass(method.type)
              : 'border-gray-200 bg-white text-gray-700 shadow-sm hover:-translate-y-0.5 hover:border-gray-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500',
        ]"
        @click="method.available && emit('select', method.type)"
      >
        <span class="flex min-w-0 items-center gap-3">
          <span :class="['flex shrink-0 items-center justify-center rounded-2xl border', props.compact ? 'h-9 w-9' : 'h-11 w-11', methodIconClass(method.type, method.available)]">
            <img :src="methodIcon(method.type)" :alt="methodLabel(method)" :class="props.compact ? 'h-5 w-5 object-contain' : 'h-7 w-7 object-contain'" />
          </span>
          <span class="min-w-0">
            <span class="block truncate text-sm font-bold text-gray-950 dark:text-white">{{ methodLabel(method) }}</span>
            <span
              v-if="method.fee_rate > 0"
              class="mt-0.5 block text-xs text-gray-500 dark:text-dark-400"
            >
              {{ t('payment.fee') }} {{ method.fee_rate }}%
            </span>
          </span>
        </span>
        <span
          v-if="selected === method.type && method.available"
          class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary-500 text-white shadow-sm"
        >
          <Icon name="check" size="xs" :stroke-width="2.5" />
        </span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { METHOD_ORDER, isBuiltInAlipayMethod, isBuiltInWxpayMethod } from './providerConfig'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'
import airwallexIcon from '@/assets/icons/airwallex.svg'
import paymentIcon from '@/assets/icons/payment.svg'

export interface PaymentMethodOption {
  type: string
  display_name?: string
  fee_rate: number
  available: boolean
}

const props = withDefaults(defineProps<{
  methods: PaymentMethodOption[]
  selected: string
  showLabel?: boolean
  compact?: boolean
}>(), {
  showLabel: true,
  compact: false,
})

const emit = defineEmits<{
  select: [type: string]
}>()

const { t } = useI18n()

const METHOD_ICONS: Record<string, string> = {
  alipay: alipayIcon,
  wxpay: wxpayIcon,
  stripe: stripeIcon,
  airwallex: airwallexIcon,
  credit_card: paymentIcon,
}

const gridClass = computed(() => props.compact
  ? 'grid grid-cols-1 gap-2 sm:grid-cols-2'
  : 'grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3'
)

const baseButtonClass = computed(() => props.compact
  ? 'group relative flex min-h-[60px] items-center justify-between gap-3 rounded-xl border p-2.5 text-left transition-all active:scale-[0.99]'
  : 'group relative flex min-h-[74px] items-center justify-between gap-3 rounded-xl border p-3 text-left transition-all active:scale-[0.99]'
)

const sortedMethods = computed(() => {
  const order: readonly string[] = METHOD_ORDER
  return [...props.methods].sort((a, b) => {
    const ai = order.indexOf(a.type)
    const bi = order.indexOf(b.type)
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })
})

function methodIcon(type: string): string {
  if (isBuiltInAlipayMethod(type)) return METHOD_ICONS.alipay
  if (isBuiltInWxpayMethod(type)) return METHOD_ICONS.wxpay
  if (type === 'airwallex') return METHOD_ICONS.airwallex
  return METHOD_ICONS[type] || paymentIcon
}

function methodLabel(method: PaymentMethodOption): string {
  return method.display_name || t(`payment.methods.${method.type}`, method.type)
}

function methodSelectedClass(type: string): string {
  if (isBuiltInAlipayMethod(type)) return 'border-[#02A9F1] bg-blue-50 text-gray-900 shadow-lg shadow-blue-500/10 ring-1 ring-blue-500/15 dark:bg-blue-950 dark:text-gray-100'
  if (isBuiltInWxpayMethod(type)) return 'border-[#09BB07] bg-green-50 text-gray-900 shadow-lg shadow-green-500/10 ring-1 ring-green-500/15 dark:bg-green-950 dark:text-gray-100'
  if (type === 'stripe') return 'border-[#676BE5] bg-indigo-50 text-gray-900 shadow-lg shadow-indigo-500/10 ring-1 ring-indigo-500/15 dark:bg-indigo-950 dark:text-gray-100'
  if (type === 'airwallex') return 'border-[#FF6B3D] bg-orange-50 text-gray-900 shadow-lg shadow-orange-500/10 ring-1 ring-orange-500/15 dark:border-[#FF8E3C] dark:bg-orange-950 dark:text-gray-100'
  return 'border-primary-500 bg-primary-50 text-gray-900 shadow-lg shadow-primary-500/10 ring-1 ring-primary-500/15 dark:bg-primary-950 dark:text-gray-100'
}

function methodIconClass(type: string, available: boolean): string {
  if (!available) return 'border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900'
  if (isBuiltInAlipayMethod(type)) return 'border-blue-100 bg-white dark:border-blue-900/60 dark:bg-blue-950/40'
  if (isBuiltInWxpayMethod(type)) return 'border-green-100 bg-white dark:border-green-900/60 dark:bg-green-950/40'
  if (type === 'stripe') return 'border-indigo-100 bg-white dark:border-indigo-900/60 dark:bg-indigo-950/40'
  if (type === 'airwallex') return 'border-orange-100 bg-white dark:border-orange-900/60 dark:bg-orange-950/40'
  return 'border-primary-100 bg-white dark:border-primary-900/60 dark:bg-primary-950/40'
}
</script>