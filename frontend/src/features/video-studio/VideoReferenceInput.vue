<template>
  <div class="space-y-2">
    <div class="flex items-center justify-between gap-2">
      <label :for="id" class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ label }}</label>
      <span class="text-[11px] text-gray-400">{{ modelValue.length }}/{{ max }} · 20MB</span>
    </div>
    <ul v-if="modelValue.length" class="space-y-1">
      <li v-for="(item, index) in modelValue" :key="index" class="flex items-center gap-2 rounded-md bg-gray-50 px-2 py-1.5 text-xs dark:bg-dark-800">
        <span class="min-w-0 flex-1 truncate" :title="item.file?.name || item.url">{{ item.file?.name || item.url }}</span>
        <button type="button" :disabled="disabled" :aria-label="t('videoStudio.removeReference')" class="text-gray-500 hover:text-red-500" @click="remove(index)">×</button>
      </li>
    </ul>
    <div v-if="modelValue.length < max" class="flex items-center gap-1.5">
      <input :id="id" v-model="url" type="text" :disabled="disabled" class="input min-w-0 flex-1 text-xs" :placeholder="t('videoStudio.referenceUrl')" @keydown.enter.prevent="addURL" />
      <button type="button" :disabled="disabled || !url.trim()" class="shrink-0 rounded-md border border-gray-200 px-2 py-2 text-xs dark:border-dark-600" @click="addURL">{{ t('videoStudio.addReference') }}</button>
      <button type="button" :disabled="disabled" class="shrink-0 rounded-md border border-gray-200 px-2 py-2 text-xs dark:border-dark-600" @click="input?.click()">{{ t('videoStudio.uploadReference') }}</button>
    </div>
    <input ref="input" type="file" class="sr-only" :accept="mediaType === 'image' ? 'image/png,image/jpeg,image/webp' : 'video/mp4,video/webm,video/quicktime'" :multiple="max > 1" :disabled="disabled" @change="addFiles" />
    <p v-if="error" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { VideoReferenceInputValue } from './types'

const props = defineProps<{
  id: string
  label: string
  mediaType: 'image' | 'video'
  max: number
  disabled?: boolean
  modelValue: VideoReferenceInputValue[]
}>()
const emit = defineEmits<{ 'update:modelValue': [VideoReferenceInputValue[]] }>()
const { t } = useI18n()
const input = ref<HTMLInputElement | null>(null)
const url = ref('')
const error = ref('')
const maxBytes = 20 * 1024 * 1024

function addURL() {
  if (props.disabled || props.modelValue.length >= props.max) return
  const value = url.value.trim()
  const mediaPrefix = `data:${props.mediaType}/`
  let valid = value.startsWith(mediaPrefix)
  if (!valid) {
    try { const parsed = new URL(value); valid = ['http:', 'https:'].includes(parsed.protocol) && !parsed.username && !parsed.password } catch { /* Invalid input. */ }
  }
  if (!valid) { error.value = t('videoStudio.referenceUrlError'); return }
  if (value.startsWith('data:') && value.length > Math.ceil(maxBytes * 4 / 3) + 1024) {
    error.value = t('videoStudio.referenceSizeError'); return
  }
  emit('update:modelValue', [...props.modelValue, { url: value }])
  url.value = ''
  error.value = ''
}

function addFiles(event: Event) {
  const element = event.target as HTMLInputElement
  const files = Array.from(element.files || [])
  element.value = ''
  if (props.disabled || files.length === 0) return
  if (props.modelValue.length + files.length > props.max) {
    error.value = t('videoStudio.referenceCountError', { max: props.max }); return
  }
  const allowed = props.mediaType === 'image'
    ? ['image/png', 'image/jpeg', 'image/webp'] : ['video/mp4', 'video/webm', 'video/quicktime']
  if (files.some(file => !allowed.includes(file.type))) {
    error.value = t(props.mediaType === 'image' ? 'videoStudio.sourceImageTypeError' : 'videoStudio.referenceVideoTypeError'); return
  }
  if (files.some(file => file.size > maxBytes)) { error.value = t('videoStudio.referenceSizeError'); return }
  emit('update:modelValue', [...props.modelValue, ...files.map(file => ({ file, url: '' }))])
  error.value = ''
}

function remove(index: number) {
  if (!props.disabled) emit('update:modelValue', props.modelValue.filter((_, position) => position !== index))
}
</script>
