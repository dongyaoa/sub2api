<template>
  <section :class="title ? 'border-t border-gray-200 pt-5 dark:border-dark-700' : ''">
    <h4 v-if="title" class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ title }}</h4>
    <dl class="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="row in rows" :key="row.label" :class="row.wide ? 'sm:col-span-2 lg:col-span-3' : ''">
        <dt class="text-xs font-medium text-gray-500">{{ row.label }}</dt>
        <dd class="mt-1 break-words text-sm text-gray-800 dark:text-gray-200" :class="row.mono ? 'font-mono text-xs' : ''">{{ row.value }}</dd>
      </div>
    </dl>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  title?: string
  rows: Array<{ label: string; value: string; mono?: boolean; wide?: boolean }>
}>()
</script>
