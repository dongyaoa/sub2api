<template>
  <DataTable
    :columns="columns"
    :data="orders"
    :loading="loading"
    :class="{ 'compact-order-table': compact }"
  >
    <template #cell-id="{ value }">
      <span class="font-mono text-sm">#{{ value }}</span>
    </template>
    <template #cell-out_trade_no="{ value, row }">
      <div v-if="compact" class="flex min-w-0 items-baseline justify-end gap-3 md:justify-start">
        <span class="shrink-0 text-xs tabular-nums text-gray-400 dark:text-dark-400" :title="t('payment.orders.orderId')">
          <span class="sr-only">{{ t('payment.orders.orderId') }}</span>#{{ row.id }}
        </span>
        <span class="min-w-0 break-all font-mono text-[13px] leading-relaxed text-gray-700 dark:text-gray-200">{{ value }}</span>
      </div>
      <span v-else class="text-sm text-gray-900 dark:text-white">{{ value }}</span>
    </template>
    <template v-if="showUser" #cell-user_email="{ value, row }">
      <div class="text-sm">
        <span class="text-gray-900 dark:text-white">{{ value || row.user_name || '#' + row.user_id }}</span>
        <span v-if="row.user_notes" class="ml-1 text-xs text-gray-400">({{ row.user_notes }})</span>
      </div>
    </template>
    <template #cell-pay_amount="{ value, row }">
      <div class="text-sm" :class="compact ? 'min-w-0 tabular-nums' : ''">
        <span class="text-gray-900 dark:text-white" :class="compact ? 'font-semibold' : 'font-medium'">{{ paymentAmountSymbol(row) }}{{ value.toFixed(2) }}</span>
        <span v-if="row.fee_rate > 0" class="text-xs text-gray-400" :class="compact ? 'mt-1 block dark:text-dark-400' : 'ml-1'" :title="t('payment.orders.fee') + ': ' + row.fee_rate + '%'">
          ({{ t('payment.orders.fee') }} {{ row.fee_rate }}%)
        </span>
        <div v-if="row.amount !== row.pay_amount || (row.bonus_amount ?? 0) > 0" class="text-xs text-gray-500" :class="compact ? 'mt-1 leading-relaxed dark:text-dark-400' : ''">
          {{ t('payment.orders.creditedAmount') }}: {{ creditedAmountSymbol }}{{ row.amount.toFixed(2) }}
          <span v-if="(row.bonus_amount ?? 0) > 0" class="text-amber-600 dark:text-amber-400" :class="compact ? 'mt-0.5 block' : 'ml-1'">
            ({{ t('payment.orders.bonusIncluded', { amount: creditedAmountSymbol + (row.bonus_amount ?? 0).toFixed(2) }) }})
          </span>
        </div>
      </div>
    </template>
    <template #cell-payment_type="{ value }">
      <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.methods.' + value, value) }}</span>
    </template>
    <template #cell-status="{ value }">
      <OrderStatusBadge :status="value" />
    </template>
    <template #cell-created_at="{ value }">
      <time v-if="compact" :datetime="value" class="text-xs tabular-nums leading-relaxed text-gray-500 dark:text-dark-400">{{ formatDate(value) }}</time>
      <span v-else class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(value) }}</span>
    </template>
    <template #cell-actions="{ row }">
      <slot name="actions" :row="row" />
    </template>
    <template v-if="compact" #empty>
      <div class="flex flex-col items-center px-4 py-6">
        <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-50 text-primary-500 dark:bg-primary-900/20 dark:text-primary-400">
          <Icon name="document" size="lg" />
        </div>
        <p class="text-base font-semibold text-gray-900 dark:text-white">{{ t('payment.orders.empty') }}</p>
        <p class="mt-2 max-w-xs text-sm leading-relaxed text-gray-500 dark:text-dark-400">{{ t('payment.orders.emptyHint') }}</p>
      </div>
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import type { Column } from '@/components/common/types'
import DataTable from '@/components/common/DataTable.vue'
import Icon from '@/components/icons/Icon.vue'
import OrderStatusBadge from '@/components/payment/OrderStatusBadge.vue'
import { currencySymbol } from '@/components/payment/currency'

const { t } = useI18n()

const props = defineProps<{
  orders: PaymentOrder[]
  loading: boolean
  showUser?: boolean
  compact?: boolean
}>()

function formatDate(dateStr: string) { return new Date(dateStr).toLocaleString() }

const creditedAmountSymbol = currencySymbol('USD')

function paymentAmountSymbol(order: PaymentOrder): string {
  return currencySymbol(order.currency)
}

const columns = computed((): Column[] => {
  const cols: Column[] = [
    ...(!props.compact ? [{ key: 'id', label: t('payment.orders.orderId') }] : []),
    { key: 'out_trade_no', label: t('payment.orders.orderNo') },
  ]
  if (props.showUser) {
    cols.push({ key: 'user_email', label: t('payment.admin.colUser') })
  }
  cols.push(
    { key: 'pay_amount', label: t('payment.orders.payAmount') },
    { key: 'payment_type', label: t('payment.orders.paymentMethod') },
    { key: 'status', label: t('payment.orders.status') },
    { key: 'created_at', label: t('payment.orders.createdAt') },
    { key: 'actions', label: t('common.actions') },
  )
  if (!props.compact) return cols

  const compactWidths: Record<string, string> = {
    out_trade_no: 'w-[27%]',
    pay_amount: 'w-[15%]',
    payment_type: 'w-[12%]',
    status: 'w-[12%]',
    created_at: 'w-[22%]',
    actions: 'w-[12%]',
  }
  return cols.map(column => ({ ...column, class: compactWidths[column.key] }))
})
</script>

<style scoped>
.compact-order-table :deep(table) {
  table-layout: fixed;
  min-width: 760px;
}

.compact-order-table :deep(th),
.compact-order-table :deep(td) {
  padding-left: 1rem;
  padding-right: 1rem;
  white-space: normal;
  overflow-wrap: anywhere;
}

.compact-order-table :deep(td) {
  padding-top: 0.875rem;
  padding-bottom: 0.875rem;
}

.compact-order-table :deep(th:first-child),
.compact-order-table :deep(td:first-child) {
  padding-left: 1.5rem;
}

.compact-order-table :deep(th:last-child),
.compact-order-table :deep(td:last-child) {
  padding-left: 0.5rem;
  padding-right: 1rem;
}
</style>
