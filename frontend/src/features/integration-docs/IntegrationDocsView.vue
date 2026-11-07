<template>
  <AppLayout>
    <div class="integration-docs mx-auto max-w-[1440px]">
      <header class="mb-5 flex flex-wrap items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-400">
            <Icon name="book" size="lg" aria-hidden="true" />
          </span>
          <div>
            <h2 class="text-xl font-semibold tracking-tight text-gray-900 dark:text-white">{{ t('integrationDocs.title') }}</h2>
            <p class="mt-1 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ localText('从对话到多媒体，找到你的接入方式。', 'Find your integration guide, from chat to multimedia.') }}</p>
          </div>
        </div>
        <span class="hidden text-xs text-gray-400 sm:block">{{ localText('接口说明 · 客户端配置 · 调用示例', 'API reference · Client setup · Examples') }}</span>
      </header>

      <nav class="docs-category-tabs" :aria-label="localText('快速切换栏目', 'Switch documentation category')">
        <router-link
          v-for="category in docCategories"
          :key="category.id"
          :to="sectionLocation(category.id, category.sections[0]?.id || '')"
          class="docs-category-tab"
          :class="{ 'docs-category-tab-active': activeCategory.id === category.id }"
          :aria-current="activeCategory.id === category.id ? 'true' : undefined"
          @click="onCategoryClick(category.id)"
        >
          <svg class="h-4 w-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" :d="categoryIcons[category.id]" /></svg>
          <span>{{ categoryTitle(category) }}</span>
          <span class="docs-category-count">{{ category.sections.length }}</span>
        </router-link>
      </nav>

      <div class="grid items-start gap-5 xl:grid-cols-[240px_minmax(0,1fr)]">
        <aside class="docs-sidebar min-w-0 rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900 xl:sticky xl:top-20" :aria-label="localText('文档目录', 'Documentation contents')">
          <button type="button" class="flex w-full items-center justify-between p-4 text-sm font-medium text-gray-800 dark:text-gray-100 xl:hidden" :aria-expanded="mobileContentsOpen" aria-controls="docs-contents" @click="mobileContentsOpen = !mobileContentsOpen">
            <span class="flex items-center gap-2"><Icon name="menu" size="sm" />{{ localText('文档目录', 'Contents') }}<span class="font-normal text-gray-400">/ {{ categoryTitle(activeCategory) }}</span></span>
            <Icon name="chevronDown" size="sm" :class="{ 'rotate-180': mobileContentsOpen }" />
          </button>
          <div id="docs-contents" :class="mobileContentsOpen ? 'block' : 'hidden xl:block'">
            <div class="border-b border-gray-100 p-3 dark:border-dark-700">
              <label class="relative block">
                <Icon name="search" size="sm" class="pointer-events-none absolute left-3 top-3 text-gray-400" />
                <input v-model="search" type="search" class="w-full rounded-lg border border-gray-200 bg-gray-50 py-2.5 pl-9 pr-2 text-xs text-gray-800 outline-none focus:border-primary-400 focus:ring-2 focus:ring-primary-500/15 dark:border-dark-700 dark:bg-dark-950 dark:text-gray-200" :aria-label="localText('搜索文档', 'Search documentation')" :placeholder="localText('搜索标题、模型或接口', 'Search titles, models or APIs')" />
              </label>
              <p v-if="search.trim()" class="mt-2 px-1 text-xs text-gray-500" role="status">{{ localText(`找到 ${resultCount} 个章节`, `${resultCount} sections found`) }}</p>
            </div>
            <div class="docs-nav-scroll p-3">
              <nav class="space-y-3" :aria-label="localText('接入文档栏目', 'Documentation categories')">
                <section v-for="category in visibleCategories" :key="category.id">
                  <div class="mb-1 flex items-center gap-1">
                    <button type="button" class="flex min-w-0 flex-1 items-center gap-2 rounded-lg px-2 py-2 text-left text-sm font-semibold text-gray-800 hover:bg-gray-50 dark:text-gray-100 dark:hover:bg-dark-800" :aria-expanded="isExpanded(category.id)" :aria-controls="`docs-category-${category.id}`" @click="toggleCategory(category.id)">
                      <svg class="h-4 w-4 shrink-0 text-primary-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path stroke-linecap="round" stroke-linejoin="round" :d="categoryIcons[category.id]" /></svg>
                      <span class="flex-1">{{ categoryTitle(category) }}</span>
                      <Icon name="chevronDown" size="xs" class="text-gray-400 transition-transform" :class="{ '-rotate-90': !isExpanded(category.id) }" />
                    </button>
                    <button type="button" class="docs-icon-button" :aria-label="exportLabel(category)" :title="exportLabel(category)" @click="exportCategory(category.id)"><Icon name="download" size="sm" /></button>
                  </div>
                  <div v-show="isExpanded(category.id)" :id="`docs-category-${category.id}`" class="ml-4 border-l border-gray-100 pl-2 dark:border-dark-700">
                    <router-link v-for="section in category.sections" :key="section.id" :to="sectionLocation(category.id, section.id)" class="docs-section-link" :class="{ 'docs-section-link-active': isSelected(category.id, section.id) }" :aria-current="isSelected(category.id, section.id) ? 'page' : undefined" @click="onSectionClick">
                      {{ section.title }}
                    </router-link>
                  </div>
                </section>
              </nav>
              <p v-if="!visibleCategories.length" class="px-2 py-6 text-center text-sm leading-6 text-gray-500 dark:text-gray-400">{{ localText('没有找到相关内容，试试模型名称或接口路径。', 'No matches. Try a model name or an API path.') }}</p>
              <p class="mx-2 mt-5 border-t border-gray-100 pt-3 text-[11px] leading-5 text-gray-400 dark:border-dark-700">{{ localText('点击栏目旁的下载图标，导出该栏目完整 Markdown。', 'Download the complete Markdown using the icon beside each category.') }}</p>
            </div>
          </div>
        </aside>

        <article ref="articleRef" class="min-w-0 scroll-mt-20 rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900" aria-labelledby="docs-section-title">
          <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:px-8">
            <div class="flex min-w-0 items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <span class="font-medium text-primary-600 dark:text-primary-400">{{ categoryTitle(activeCategory) }}</span><Icon name="chevronRight" size="xs" /><span>{{ localText(`第 ${sectionIndex + 1} / ${activeCategory.sections.length} 节`, `Section ${sectionIndex + 1} / ${activeCategory.sections.length}`) }}</span>
            </div>
            <button type="button" class="inline-flex items-center gap-1.5 rounded-lg bg-primary-50 px-2.5 py-2 text-xs font-medium text-primary-700 transition-colors hover:bg-primary-100 dark:bg-primary-500/10 dark:text-primary-300 dark:hover:bg-primary-500/20" @click="exportCategory(activeCategory.id)"><Icon name="download" size="sm" />{{ localText('导出本栏目 Markdown', 'Export category Markdown') }}</button>
          </div>
          <div class="docs-reading-column">
            <h1 id="docs-section-title" tabindex="-1" class="mb-2 break-words text-2xl font-semibold leading-snug tracking-tight text-gray-900 outline-none dark:text-white sm:text-[28px]">{{ activeSection.title }}</h1>
            <p class="mb-7 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ activeCategory.description }}</p>
            <div class="docs-prose">
              <template v-for="(block, index) in blocks" :key="`${activeCategory.id}-${activeSection.id}-${index}`">
                <div v-if="block.type === 'html'" v-html="block.html"></div>
                <div v-else class="docs-code-block">
                  <div class="flex items-center justify-between border-b border-gray-200 bg-gray-100/70 px-4 py-2 dark:border-dark-700 dark:bg-dark-800">
                    <span class="font-mono text-[11px] text-gray-500 dark:text-gray-400">{{ block.language }}</span>
                    <button type="button" class="inline-flex items-center gap-1.5 rounded px-1.5 py-1 text-xs text-gray-500 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400" :aria-label="localText(`复制 ${block.language} 代码`, `Copy ${block.language} code`)" @click="copyToClipboard(block.text)"><Icon name="copy" size="xs" />{{ localText('复制', 'Copy') }}</button>
                  </div>
                  <pre tabindex="0"><code>{{ block.text }}</code></pre>
                </div>
              </template>
            </div>
            <footer class="mt-10 grid grid-cols-2 gap-3 border-t border-gray-100 pt-5 dark:border-dark-700">
              <router-link v-if="previousSection" :to="sectionLocation(activeCategory.id, previousSection.id)" class="docs-page-link" @click="onSectionClick"><span class="mb-1 text-xs text-gray-400">{{ localText('上一节', 'Previous') }}</span><span class="flex items-center gap-1 text-sm"><Icon name="arrowLeft" size="xs" class="shrink-0" />{{ previousSection.title }}</span></router-link>
              <router-link v-if="nextSection" :to="sectionLocation(activeCategory.id, nextSection.id)" class="docs-page-link col-start-2 items-end text-right" @click="onSectionClick"><span class="mb-1 text-xs text-gray-400">{{ localText('下一节', 'Next') }}</span><span class="flex items-center gap-1 text-sm">{{ nextSection.title }}<Icon name="arrowRight" size="xs" class="shrink-0" /></span></router-link>
            </footer>
          </div>
        </article>
      </div>
      <p class="mt-5 text-center text-xs leading-5 text-gray-400">{{ localText('模型可用性与价格以当前 API Key 所属分组的模型广场为准。', 'Model availability and pricing depend on the group assigned to your API key.') }}</p>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { docCategories, filterCategories, resolveSelection, type DocCategory } from './catalog'
import { renderDocument } from './markdown'
import './docs.css'

const route = useRoute()
const { t, locale } = useI18n()
const { copyToClipboard } = useClipboard()
const localText = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const categoryTitle = (category: DocCategory) => localText(category.title, category.englishTitle)
const search = ref('')
const mobileContentsOpen = ref(false)
const expanded = ref<Record<string, boolean>>(Object.fromEntries(docCategories.map(category => [category.id, true])))
const articleRef = ref<HTMLElement | null>(null)
const selection = computed(() => resolveSelection(route.query.category, route.query.section))
const activeCategory = computed(() => selection.value.category)
const activeSection = computed(() => selection.value.section)
const sectionIndex = computed(() => activeCategory.value.sections.indexOf(activeSection.value))
const previousSection = computed(() => activeCategory.value.sections[sectionIndex.value - 1])
const nextSection = computed(() => activeCategory.value.sections[sectionIndex.value + 1])
const visibleCategories = computed(() => filterCategories(search.value))
const resultCount = computed(() => visibleCategories.value.reduce((sum, category) => sum + category.sections.length, 0))
const blocks = computed(() => renderDocument(activeSection.value.markdown))
const categoryIcons = {
  chat: 'M7.5 8.25h9m-9 3.75h6m-7.5 9 3.75-3.75H18a3 3 0 003-3V6a3 3 0 00-3-3H6a3 3 0 00-3 3v8.25a3 3 0 003 3V21z',
  image: 'M3 5a2 2 0 012-2h14a2 2 0 012 2v14a2 2 0 01-2 2H5a2 2 0 01-2-2V5zM3 16l5-5 4 4 3-3 6 6M15 8h.01',
  video: 'M15 10l5-3v10l-5-3M4 6h9a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V8a2 2 0 012-2z',
  speech: 'M12 15a3 3 0 003-3V5a3 3 0 00-6 0v7a3 3 0 003 3zM5 10v2a7 7 0 0014 0v-2M12 19v3m-4 0h8'
}

const sectionLocation = (category: string, section: string) => ({ path: '/integration-docs', query: { category, section } })
const isSelected = (category: string, section: string) => activeCategory.value.id === category && activeSection.value.id === section
const isExpanded = (category: string) => search.value.trim() !== '' || expanded.value[category]
const exportLabel = (category: DocCategory) => localText(`导出${category.title} Markdown`, `Export ${category.englishTitle} Markdown`)

function toggleCategory(category: string) {
  // Leaving search restores ordinary expand/collapse behavior.
  if (search.value.trim()) {
    search.value = ''
    expanded.value[category] = false
  } else {
    expanded.value[category] = !expanded.value[category]
  }
}

function exportCategory(id: string) {
  // Always export the complete source, including chapters hidden by search.
  const category = docCategories.find(item => item.id === id)!
  saveAs(new Blob([category.markdown], { type: 'text/markdown;charset=utf-8' }), category.filename)
}

function onSectionClick() {
  mobileContentsOpen.value = false
}

function onCategoryClick(category: string) {
  search.value = ''
  expanded.value[category] = true
  mobileContentsOpen.value = false
}

watch(() => [activeCategory.value.id, activeSection.value.id], async () => {
  expanded.value[activeCategory.value.id] = true
  await nextTick()
  articleRef.value?.querySelector<HTMLElement>('#docs-section-title')?.focus({ preventScroll: true })
})
</script>
