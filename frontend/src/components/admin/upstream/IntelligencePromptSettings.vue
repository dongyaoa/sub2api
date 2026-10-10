<template>
  <section class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-700" data-testid="local-prompt-settings">
    <div>
      <label for="intelligence-custom-prompt" class="input-label">{{ t('intelligenceMonitor.promptSettings.custom') }}</label>
      <textarea id="intelligence-custom-prompt" :value="customPrompt || ''" class="input min-h-[100px]" maxlength="8000" :disabled="disabled" :placeholder="PELICAN_PROMPT" aria-describedby="intelligence-custom-prompt-hint" @input="updateCustomPrompt" />
      <p id="intelligence-custom-prompt-hint" class="mt-2 text-xs leading-5 text-gray-500">{{ t('intelligenceMonitor.promptSettings.customHint') }}</p>
    </div>
    <details class="rounded-lg bg-gray-50 p-3 dark:bg-dark-900/50">
      <summary class="cursor-pointer text-xs font-medium text-gray-500">{{ t('intelligenceMonitor.promptSettings.default') }}</summary>
      <p class="mt-2 whitespace-pre-wrap break-words text-xs leading-6 text-gray-600 dark:text-dark-300">{{ PELICAN_PROMPT }}</p>
    </details>
    <div v-if="allowChannels" class="border-t border-gray-100 pt-4 dark:border-dark-700">
      <button type="button" class="flex w-full items-center justify-between gap-3 text-left text-sm font-medium text-gray-800 dark:text-gray-200" :aria-expanded="channelsExpanded" aria-controls="intelligence-channel-prompts" data-testid="toggle-channel-prompts" @click="toggleChannels">
        <span>{{ t('intelligenceMonitor.promptSettings.channels') }}<span v-if="configuredCount" class="ml-2 text-xs font-normal text-primary-600 dark:text-primary-300">{{ t('intelligenceMonitor.promptSettings.configuredCount', { count: configuredCount }) }}</span></span>
        <Icon :name="channelsExpanded ? 'chevronDown' : 'chevronRight'" size="sm" />
      </button>
      <p class="mt-2 text-xs leading-5 text-gray-500">{{ t('intelligenceMonitor.promptSettings.priority') }}</p>
      <div v-if="channelsExpanded" id="intelligence-channel-prompts" class="mt-3 space-y-3">
        <p v-if="!groupId" class="text-xs text-gray-500">{{ t('intelligenceMonitor.promptSettings.selectGroup') }}</p>
        <template v-else>
          <div class="flex items-center justify-between gap-3">
            <span class="text-xs text-gray-500">{{ t('intelligenceMonitor.promptSettings.channelHint') }}</span>
            <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="loading || disabled" data-testid="refresh-prompt-channels" @click="loadChannels"><Icon name="refresh" size="sm" class="mr-1.5" :class="loading && 'animate-spin'" />{{ t('intelligenceMonitor.refresh') }}</button>
          </div>
          <p v-if="loading" role="status" class="text-xs text-gray-500">{{ t('intelligenceMonitor.promptSettings.loading') }}</p>
          <p v-if="loadError" role="alert" class="text-xs text-rose-500">{{ loadError }}</p>
          <p v-if="loaded && !rows.length" class="text-xs text-gray-500">{{ t('intelligenceMonitor.promptSettings.empty') }}</p>
          <div v-if="rows.length" class="max-h-[28rem] space-y-2 overflow-y-auto pr-1">
            <details v-for="channel in rows" :key="channel.account_id" :open="Boolean(promptFor(channel.account_id))" class="rounded-lg border border-gray-200 px-3 py-2.5 dark:border-dark-600" :data-channel-id="channel.account_id">
              <summary class="cursor-pointer text-xs text-gray-700 dark:text-gray-200">
                <span class="font-medium">{{ channel.name }}</span><span class="ml-2 text-[10px] text-gray-400">#{{ channel.account_id }}</span>
                <span class="ml-2 text-[10px]" :class="promptFor(channel.account_id).trim() ? 'text-primary-600 dark:text-primary-300' : 'text-gray-400'">{{ t(promptFor(channel.account_id).trim() ? 'intelligenceMonitor.promptSettings.overridden' : 'intelligenceMonitor.promptSettings.inherited') }}</span>
              </summary>
              <p v-if="channel.unavailable" class="mt-2 text-xs text-amber-600 dark:text-amber-300">{{ t('intelligenceMonitor.promptSettings.unavailable') }}</p>
              <div class="mt-3">
                <label :for="`intelligence-channel-prompt-${channel.account_id}`" class="input-label">{{ t('intelligenceMonitor.promptSettings.channelPrompt', { name: channel.name }) }}</label>
                <textarea :id="`intelligence-channel-prompt-${channel.account_id}`" :value="promptFor(channel.account_id)" class="input min-h-[100px]" maxlength="8000" :disabled="disabled" :placeholder="inheritedPrompt" @input="updateChannelPrompt(channel.account_id, $event)" />
                <button v-if="promptFor(channel.account_id)" type="button" class="mt-2 text-xs text-gray-500 hover:text-primary-600 dark:hover:text-primary-300" :disabled="disabled" :data-clear-channel="channel.account_id" @click="setChannelPrompt(channel.account_id, '')">{{ t('intelligenceMonitor.promptSettings.clear') }}</button>
              </div>
            </details>
          </div>
        </template>
      </div>
    </div>
    <p v-if="validationError" role="alert" class="text-xs text-rose-500">{{ validationError }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { intelligenceMonitorAPI, PELICAN_PROMPT, type IntelligenceChannelPrompt, type IntelligenceLocalChannel } from '@/api/admin/intelligenceMonitor'

const props = withDefaults(defineProps<{ groupId?: number | null; customPrompt?: string; channelPrompts?: IntelligenceChannelPrompt[]; disabled?: boolean; allowChannels?: boolean }>(), { allowChannels: true })
const emit = defineEmits<{ 'update:customPrompt': [value: string]; 'update:channelPrompts': [value: IntelligenceChannelPrompt[]] }>()
const { t } = useI18n()
const channels = ref<IntelligenceLocalChannel[]>([]), loading = ref(false), loaded = ref(false), loadError = ref(''), validationError = ref('')
const channelsExpanded = ref(Boolean(props.channelPrompts?.length))
let controller: AbortController | undefined
const configuredCount = computed(() => (props.channelPrompts || []).filter(item => item.prompt.trim()).length)
const inheritedPrompt = computed(() => props.customPrompt?.trim() || PELICAN_PROMPT)
const rows = computed(() => {
  const result = channels.value.map(channel => ({ ...channel, unavailable: false }))
  const availableIDs = new Set(result.map(channel => channel.account_id))
  for (const item of props.channelPrompts || []) {
    if (!availableIDs.has(item.account_id)) {
      result.push({ account_id: item.account_id, name: t('intelligenceMonitor.promptSettings.savedChannel'), platform: '', type: '', status: '', unavailable: loaded.value })
      availableIDs.add(item.account_id)
    }
  }
  return result
})
function promptFor(accountID: number) { return props.channelPrompts?.find(item => item.account_id === accountID)?.prompt || '' }
function updateCustomPrompt(event: Event) {
  validationError.value = ''
  emit('update:customPrompt', (event.target as HTMLTextAreaElement).value)
}
function updateChannelPrompt(accountID: number, event: Event) { setChannelPrompt(accountID, (event.target as HTMLTextAreaElement).value) }
function setChannelPrompt(accountID: number, prompt: string) {
  validationError.value = ''
  const next = (props.channelPrompts || []).filter(item => item.account_id !== accountID)
  if (prompt) next.push({ account_id: accountID, prompt })
  emit('update:channelPrompts', next)
}
function toggleChannels() {
  channelsExpanded.value = !channelsExpanded.value
  if (channelsExpanded.value && !loaded.value && !loading.value) void loadChannels()
}
async function loadChannels() {
  controller?.abort()
  if (!props.allowChannels || !props.groupId) return
  const current = new AbortController(); controller = current
  const groupID = props.groupId
  loading.value = true; loadError.value = ''
  try {
    const result = await intelligenceMonitorAPI.localChannels(groupID, current.signal)
    if (current.signal.aborted || props.groupId !== groupID) return
    channels.value = result.items
    loaded.value = true
  } catch {
    if (!current.signal.aborted && props.groupId === groupID) loadError.value = t('intelligenceMonitor.promptSettings.loadFailed')
  } finally { if (!current.signal.aborted) loading.value = false }
}
watch([() => props.groupId, () => props.allowChannels], () => {
  controller?.abort(); channels.value = []; loaded.value = false; loading.value = false; loadError.value = ''; validationError.value = ''
  if (channelsExpanded.value) void loadChannels()
}, { immediate: true })
function validate() {
  const prompts = [props.customPrompt || '', ...(props.allowChannels ? props.channelPrompts || [] : []).map(item => item.prompt)].map(prompt => prompt.trim())
  const sizes = prompts.map(prompt => Array.from(prompt).length)
  validationError.value = prompts.some(prompt => prompt.includes('\0')) ? t('intelligenceMonitor.promptSettings.invalidCharacters')
    : sizes.some(size => size > 8000) ? t('intelligenceMonitor.promptSettings.tooLong')
    : (props.allowChannels && configuredCount.value > 200) || sizes.reduce((sum, size) => sum + size, 0) > 64000 ? t('intelligenceMonitor.promptSettings.tooMany')
      : ''
  return !validationError.value
}
defineExpose({ validate })
onBeforeUnmount(() => controller?.abort())
</script>
