<template>
  <div class="space-y-5">
    <section data-testid="profile-overview-hero" class="card">
      <div class="grid items-center gap-5 p-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
        <div class="flex min-w-0 items-center gap-3">
          <div class="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-primary-600 text-xl font-semibold text-white">
            <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" class="h-full w-full object-cover">
            <span v-else>{{ avatarInitial }}</span>
          </div>
          <div class="min-w-0 flex-1">
            <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white" :title="displayName">
              {{ displayName }}
            </h2>
            <p v-if="primaryEmailDisplay && primaryEmailDisplay !== displayName" class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400" :title="primaryEmailDisplay">
              {{ primaryEmailDisplay }}
            </p>
            <div class="mt-2 flex flex-wrap items-center gap-2">
              <span :class="['badge', user?.role === 'admin' ? 'badge-primary' : 'badge-gray']">
                {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
              </span>
              <span :class="['badge', user?.status === 'active' ? 'badge-success' : 'badge-danger']">
                {{ user?.status === 'active' ? t('common.active') : t('common.disabled') }}
              </span>
            </div>
          </div>
        </div>

        <dl class="grid grid-cols-2 gap-x-4 gap-y-3 border-t border-gray-100 pt-4 dark:border-dark-700 sm:grid-cols-3 xl:border-t-0 xl:pt-0">
          <div data-testid="profile-overview-metric-balance" class="min-w-0">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('profile.accountBalance') }}</dt>
            <dd class="mt-1.5 break-all text-xl font-semibold tabular-nums text-primary-700 dark:text-primary-400">
              {{ formatCurrency(user?.balance || 0) }}
            </dd>
          </div>
          <div data-testid="profile-overview-metric-concurrency" class="min-w-0 border-l border-gray-100 pl-4 dark:border-dark-700">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('profile.concurrencyLimit') }}</dt>
            <dd class="mt-1.5 break-all text-xl font-semibold tabular-nums text-gray-900 dark:text-white">
              {{ user?.concurrency || 0 }}
            </dd>
          </div>
          <div data-testid="profile-overview-metric-member-since" class="col-span-2 min-w-0 sm:col-span-1 sm:border-l sm:border-gray-100 sm:pl-4 dark:sm:border-dark-700">
            <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('profile.memberSince') }}</dt>
            <dd class="mt-1.5 text-base font-medium text-gray-900 dark:text-white">{{ memberSinceLabel }}</dd>
          </div>
        </dl>
      </div>
    </section>

    <div data-testid="profile-settings-grid" class="grid items-start gap-5 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_minmax(0,1fr)]">
      <section data-testid="profile-basics-panel" class="card min-w-0" aria-labelledby="profile-basics-title">
        <div class="flex items-center gap-2.5 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <Icon name="user" size="md" class="shrink-0 text-primary-500" />
          <h3 id="profile-basics-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('profile.basicsTitle') }}</h3>
        </div>
        <div data-testid="profile-main-column" class="min-w-0 p-5">
          <ProfileAvatarCard :user="user" embedded compact />
          <div class="mt-5 border-t border-gray-100 pt-5 dark:border-dark-700">
            <ProfileEditForm :initial-username="user?.username || ''" embedded compact />
          </div>
          <div v-if="sourceHints.length" class="mt-4 space-y-2 border-t border-gray-100 pt-4 dark:border-dark-700">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('profile.linkedProfileSources') }}</p>
            <p v-for="hint in sourceHints" :key="hint.key" class="flex min-w-0 items-start gap-2 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
              <Icon name="link" size="sm" class="mt-0.5 shrink-0" />
              <span class="min-w-0 break-words">{{ hint.text }}</span>
            </p>
          </div>
        </div>
      </section>

      <section data-testid="profile-auth-bindings-panel" class="card min-w-0" aria-labelledby="profile-bindings-title">
        <div class="flex items-center gap-2.5 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <Icon name="link" size="md" class="shrink-0 text-primary-500" />
          <h3 id="profile-bindings-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('profile.authBindings.title') }}</h3>
        </div>
        <div class="px-5 py-4">
          <ProfileIdentityBindingsSection
            :user="user"
            :linuxdo-enabled="linuxdoEnabled"
            :dingtalk-enabled="dingtalkEnabled"
            :oidc-enabled="oidcEnabled"
            :oidc-provider-name="oidcProviderName"
            :wechat-enabled="wechatEnabled"
            :wechat-open-enabled="wechatOpenEnabled"
            :wechat-mp-enabled="wechatMpEnabled"
            embedded
            compact
            hide-heading
          />
        </div>
      </section>

      <section data-testid="profile-side-column" class="card min-w-0 md:col-span-2 xl:col-span-1" aria-labelledby="profile-security-title">
        <div class="flex items-center gap-2.5 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
          <Icon name="shield" size="md" class="shrink-0 text-primary-500" />
          <h3 id="profile-security-title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('profile.securityTitle') }}</h3>
        </div>
        <div data-testid="profile-security-panel" class="px-5 py-4">
          <div class="divide-y divide-gray-100 dark:divide-dark-700">
            <slot name="security" />
            <slot v-if="showNotifications" name="notifications" />
          </div>
          <slot name="support" />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ProfileAvatarCard from '@/components/user/profile/ProfileAvatarCard.vue'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import type { User, UserAuthBindingStatus, UserAuthProvider, UserProfileSourceContext } from '@/types'

const props = withDefaults(defineProps<{
  user: User | null
  linuxdoEnabled?: boolean
  dingtalkEnabled?: boolean
  oidcEnabled?: boolean
  oidcProviderName?: string
  wechatEnabled?: boolean
  wechatOpenEnabled?: boolean
  wechatMpEnabled?: boolean
  showNotifications?: boolean
}>(), {
  linuxdoEnabled: false,
  dingtalkEnabled: false,
  oidcEnabled: false,
  oidcProviderName: 'OIDC',
  wechatEnabled: false,
  wechatOpenEnabled: undefined,
  wechatMpEnabled: undefined,
  showNotifications: false,
})

const { t } = useI18n()


function normalizeBindingStatus(binding: boolean | UserAuthBindingStatus | undefined): boolean | null {
  if (typeof binding === 'boolean') {
    return binding
  }
  if (!binding) {
    return null
  }
  if (typeof binding.bound === 'boolean') {
    return binding.bound
  }
  return Boolean(binding.provider_subject || binding.issuer || binding.provider_key)
}

function isEmailBound(user: User | null | undefined): boolean {
  if (typeof user?.email_bound === 'boolean') {
    return user.email_bound
  }

  const nested = user?.auth_bindings?.email ?? user?.identity_bindings?.email
  const normalized = normalizeBindingStatus(nested)
  return normalized ?? false
}

const avatarUrl = computed(() => props.user?.avatar_url?.trim() || '')
const displayName = computed(() => props.user?.username?.trim() || props.user?.email?.trim() || t('profile.user'))
const primaryEmailDisplay = computed(() => {
  const email = props.user?.email?.trim() || ''
  if (!email) {
    return ''
  }
  if (email.endsWith('.invalid') && !isEmailBound(props.user)) {
    return ''
  }
  return email
})
const avatarInitial = computed(() => displayName.value.charAt(0).toUpperCase() || 'U')
const memberSinceLabel = computed(() => {
  const raw = props.user?.created_at?.trim()
  if (!raw) {
    return '-'
  }

  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
  }).format(date)
})

const providerLabels = computed<Record<UserAuthProvider, string>>(() => ({
  email: t('profile.authBindings.providers.email'),
  linuxdo: t('profile.authBindings.providers.linuxdo'),
  dingtalk: t('profile.authBindings.providers.dingtalk'),
  oidc: t('profile.authBindings.providers.oidc', { providerName: props.oidcProviderName }),
  wechat: t('profile.authBindings.providers.wechat'),
  github: 'GitHub',
  google: 'Google'
}))

function formatCurrency(value: number): string {
  return `$${value.toFixed(2)}`
}

function normalizeProvider(value: string): UserAuthProvider | null {
  const normalized = value.trim().toLowerCase()
  if (
    normalized === 'email' ||
    normalized === 'linuxdo' ||
    normalized === 'dingtalk' ||
    normalized === 'wechat' ||
    normalized === 'github' ||
    normalized === 'google'
  ) {
    return normalized
  }
  if (normalized === 'oidc' || normalized.startsWith('oidc:') || normalized.startsWith('oidc/')) {
    return 'oidc'
  }
  return null
}

function readObjectString(source: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return ''
}

function resolveThirdPartySource(
  rawSource: string | UserProfileSourceContext | null | undefined
): { provider: UserAuthProvider; label: string } | null {
  if (!rawSource) {
    return null
  }

  if (typeof rawSource === 'string') {
    const provider = normalizeProvider(rawSource)
    if (!provider || provider === 'email') {
      return null
    }
    return {
      provider,
      label: providerLabels.value[provider]
    }
  }

  const sourceRecord = rawSource as Record<string, unknown>
  const provider = normalizeProvider(
    readObjectString(sourceRecord, 'provider', 'source', 'provider_type', 'auth_provider')
  )
  if (!provider || provider === 'email') {
    return null
  }

  const explicitLabel = readObjectString(
    sourceRecord,
    'provider_label',
    'label',
    'provider_name',
    'providerName'
  )

  return {
    provider,
    label: explicitLabel || providerLabels.value[provider]
  }
}

const sourceHints = computed(() => {
  const currentUser = props.user
  if (!currentUser) {
    return []
  }

  const hints: Array<{ key: string; text: string }> = []
  const avatarSource = resolveThirdPartySource(
    currentUser.profile_sources?.avatar ?? currentUser.avatar_source
  )
  const usernameSource = resolveThirdPartySource(
    currentUser.profile_sources?.username ??
      currentUser.profile_sources?.display_name ??
      currentUser.profile_sources?.nickname ??
      currentUser.display_name_source ??
      currentUser.username_source ??
      currentUser.nickname_source
  )

  if (avatarSource) {
    hints.push({
      key: 'avatar',
      text: t('profile.authBindings.source.avatar', { providerName: avatarSource.label })
    })
  }

  if (usernameSource) {
    hints.push({
      key: 'username',
      text: t('profile.authBindings.source.username', { providerName: usernameSource.label })
    })
  }

  return hints
})
</script>
