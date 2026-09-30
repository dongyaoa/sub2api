<template>
  <component
    :is="variant === 'dashboard' ? RouterLink : 'div'"
    v-if="visible && promotion"
    :to="variant === 'dashboard' ? '/purchase' : undefined"
    class="promotion-notice"
    :class="`promotion-notice--${variant}`"
    :data-testid="`promotion-notice-${variant}`"
  >
    <span class="promotion-notice-icon" aria-hidden="true"><Icon name="sparkles" size="sm" /></span>
    <div class="min-w-0 flex-1">
      <template v-if="variant === 'dashboard'">
        <span class="promotion-notice-title">{{ promotion.title }}</span>
        <span class="promotion-notice-copy">{{ tp('promotion.upTo', { percent: highestBonus }) }}</span>
      </template>
      <span v-else class="leading-relaxed">{{ tp('promotion.balanceBonus', { percent: highestBonus }) }}</span>
    </div>
    <span v-if="variant === 'dashboard'" class="promotion-notice-action">
      {{ tp('promotion.rechargeNow') }}<span aria-hidden="true">↗</span>
    </span>
  </component>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { RechargePromotion } from '@/types/payment'
import { isRechargePromotionActive, useRechargePromotionClock } from '@/utils/rechargePromotion'
import { rechargePromotionMessages } from './rechargePromotionMessages'

const props = withDefaults(defineProps<{ promotion?: RechargePromotion | null; variant?: 'balance' | 'dashboard' }>(), { variant: 'dashboard' })
const { t: tp } = useI18n({ useScope: 'local', messages: rechargePromotionMessages })
const now = useRechargePromotionClock()
const visible = computed(() => isRechargePromotionActive(props.promotion, now.value))
const highestBonus = computed(() => Math.max(0, ...(props.promotion?.tiers.map(tier => tier.bonus_percent) ?? [])))
</script>

<style scoped>
.promotion-notice { display: flex; align-items: center; gap: .625rem; min-width: 0; color: #926322; }
.promotion-notice--dashboard { padding: .875rem 1rem; border: 1px solid #ead8ad; border-radius: .875rem; background: linear-gradient(110deg, #fffdf7, #faf0d8 70%, #fcf7eb); box-shadow: inset 0 1px 0 #ffffffb3; transition: border-color .2s, box-shadow .2s; }
.promotion-notice--dashboard:hover { border-color: #d5b976; box-shadow: 0 3px 12px #9d762a0a; }
.promotion-notice--dashboard:focus-visible { outline: 2px solid #b48a3c; outline-offset: 3px; }
.promotion-notice--balance { margin-top: .625rem; border-top: 1px solid #d5b97630; padding-top: .5rem; gap: .375rem; font-size: .6875rem; font-weight: 500; }
.promotion-notice-icon { display: flex; flex: 0 0 auto; color: #b48a3c; }
.promotion-notice--dashboard .promotion-notice-icon { padding: .5rem; border: 1px solid #e8d6ac; border-radius: .625rem; background: #ffffff80; }
.promotion-notice-title { margin-right: .75rem; font-size: .875rem; font-weight: 600; overflow-wrap: anywhere; color: #785018; }
.promotion-notice-copy { display: inline-block; font-size: .75rem; }
.promotion-notice-action { display: flex; flex: 0 0 auto; align-items: center; gap: .375rem; font-size: .75rem; font-weight: 600; }
.dark .promotion-notice { color: #d5b87c; }
.dark .promotion-notice--dashboard { border-color: #76603a66; background: linear-gradient(110deg, #302a21, #393022 70%, #2c2822); box-shadow: inset 0 1px 0 #f8df9f08; }
.dark .promotion-notice--dashboard:hover { border-color: #9c7b43; }
.dark .promotion-notice--dashboard .promotion-notice-icon { border-color: #8a6e383d; background: #d8b2670b; }
.dark .promotion-notice-icon { color: #d0aa62; }
.dark .promotion-notice-title { color: #ead7ae; }
@media (max-width: 480px) { .promotion-notice--dashboard { padding: .75rem; gap: .5rem; } .promotion-notice-title { display: block; margin-right: 0; font-size: .8125rem; } .promotion-notice-copy { margin-top: .125rem; font-size: .6875rem; } }
</style>
