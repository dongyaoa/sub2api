import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import { createMemoryHistory, createRouter, type RouteLocationRaw, type Router } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import IntegrationDocsView from '../IntegrationDocsView.vue'
import { docCategories } from '../catalog'

const { saveAs, copyToClipboard } = vi.hoisted(() => ({
  saveAs: vi.fn(),
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

vi.mock('file-saver', () => ({ saveAs }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key, locale: ref('zh') }),
}))

const wrappers: VueWrapper[] = []

async function openDocs(location: RouteLocationRaw = '/integration-docs') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/integration-docs', component: { template: '<div />' } }],
  })
  await router.push(location)
  await router.isReady()
  const wrapper = mount(IntegrationDocsView, { global: { plugins: [router], stubs: { Icon: true } } })
  wrappers.push(wrapper)
  return { wrapper, router }
}

async function travelHistory(router: Router, direction: 'back' | 'forward') {
  await new Promise<void>((resolve) => {
    const removeHook = router.afterEach(() => {
      removeHook()
      resolve()
    })
    router[direction]()
  })
  await flushPromises()
}

async function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsText(blob)
  })
}

beforeEach(() => vi.clearAllMocks())
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
})

describe('Integration docs reading and navigation', () => {
  it('starts with chat documentation, opens all four categories and lets a reader collapse a group', async () => {
    const { wrapper } = await openDocs()
    const groups = wrapper.findAll('button[aria-controls^="docs-category-"]')
    expect(groups).toHaveLength(4)
    expect(groups[0]!.attributes('aria-controls')).toBe('docs-category-chat')
    expect(groups.every(button => button.attributes('aria-expanded') === 'true')).toBe(true)
    expect(wrapper.findAll('.docs-section-link')).toHaveLength(docCategories.reduce((sum, category) => sum + category.sections.length, 0))
    const chat = docCategories.find(category => category.id === 'chat')!
    expect(wrapper.get('#docs-section-title').text()).toBe(chat.sections[0]!.title)
    expect(wrapper.get('.docs-section-link[aria-current="page"]').attributes('href')).toContain('category=chat')

    const videoToggle = wrapper.get('button[aria-controls="docs-category-video"]')
    await videoToggle.trigger('click')
    expect(videoToggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('#docs-category-video').attributes('style')).toContain('display: none')
    await videoToggle.trigger('click')
    expect(videoToggle.attributes('aria-expanded')).toBe('true')
  })

  it('navigates chapters with links and restores the selected chapter on browser back and forward', async () => {
    const { wrapper, router } = await openDocs()
    const speech = docCategories.find(category => category.id === 'speech')!
    const target = speech.sections[1]!
    await wrapper.get('#docs-category-speech').findAll('a')[1]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ category: 'speech', section: target.id })
    expect(wrapper.get('#docs-section-title').text()).toBe(target.title)
    expect(wrapper.get('.docs-section-link[aria-current="page"]').text()).toBe(target.title)

    await travelHistory(router, 'back')
    expect(wrapper.get('#docs-section-title').text()).toBe(docCategories[0]!.sections[0]!.title)
    await travelHistory(router, 'forward')
    expect(wrapper.get('#docs-section-title').text()).toBe(target.title)
    expect(wrapper.get('button[aria-controls="docs-category-speech"]').attributes('aria-expanded')).toBe('true')
  })

  it('opens a shared chapter URL and falls back safely for missing or invalid selections', async () => {
    const video = docCategories.find(category => category.id === 'video')!
    const target = video.sections[2]!
    const { wrapper, router } = await openDocs({ path: '/integration-docs', query: { category: 'video', section: target.id } })
    expect(wrapper.get('#docs-section-title').text()).toBe(target.title)

    await router.push({ path: '/integration-docs', query: { category: 'video', section: 'missing-section' } })
    await flushPromises()
    expect(wrapper.get('#docs-section-title').text()).toBe(video.sections[0]!.title)
    await router.push({ path: '/integration-docs', query: { category: ['unknown', 'speech'], section: 'missing' } })
    await flushPromises()
    expect(wrapper.get('#docs-section-title').text()).toBe(docCategories[0]!.sections[0]!.title)
  })

  it('switches categories from the quick navigation even after an empty search', async () => {
    const { wrapper, router } = await openDocs()
    const input = wrapper.get('input[type="search"]')
    await input.setValue('nonexistent-api-model-xyz')
    await wrapper.get('.docs-category-tabs a[href*="category=video"]').trigger('click')
    await flushPromises()
    const video = docCategories.find(category => category.id === 'video')!
    expect(router.currentRoute.value.query).toEqual({ category: 'video', section: video.sections[0]!.id })
    expect(wrapper.get('#docs-section-title').text()).toBe(video.sections[0]!.title)
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(wrapper.get('#docs-category-video .docs-section-link[aria-current="page"]').text()).toBe(video.sections[0]!.title)
  })

  it('exports the complete category Markdown even when search shows only one chapter', async () => {
    const { wrapper } = await openDocs()
    await wrapper.get('input[type="search"]').setValue('Zubenelgenubi')
    expect(wrapper.findAll('.docs-section-link')).toHaveLength(1)
    expect(wrapper.get('.docs-section-link').text()).toBe('参数与音色')

    await wrapper.get('button[aria-label="导出语音合成 Markdown"]').trigger('click')
    expect(saveAs).toHaveBeenCalledOnce()
    const [blob, filename] = saveAs.mock.calls[0] as [Blob, string]
    const speech = docCategories.find(category => category.id === 'speech')!
    expect(filename).toBe(speech.filename)
    expect(blob.type).toBe('text/markdown;charset=utf-8')
    expect(await readBlob(blob)).toBe(speech.markdown)
  })

  it('exports the chat guide with desktop setup and bearer gateway authentication', async () => {
    const { wrapper } = await openDocs()
    await wrapper.get('button[aria-label="导出对话模型 Markdown"]').trigger('click')
    expect(saveAs).toHaveBeenCalledOnce()
    const [blob, filename] = saveAs.mock.calls[0] as [Blob, string]
    const chat = docCategories.find(category => category.id === 'chat')!
    const markdown = await readBlob(blob)
    expect(filename).toBe(chat.filename)
    expect(markdown).toBe(chat.markdown)
    expect(markdown).toMatch(/^## .*CC-Switch.*Codex/m)
    expect(markdown).toMatch(/^## .*Claude Desktop/m)
    expect(markdown).not.toContain('docs.juheq.cn')
    expect(markdown).toContain('| **Gateway auth scheme** | `bearer` |')
    expect(markdown).not.toContain('Skip login-mode chooser')
  })

  it('shows empty search feedback and restores the directory after clearing the search', async () => {
    const { wrapper } = await openDocs()
    const input = wrapper.get('input[type="search"]')
    await input.setValue('nonexistent-api-model-xyz')
    expect(wrapper.findAll('.docs-section-link')).toHaveLength(0)
    expect(wrapper.text()).toContain('没有找到相关内容')
    expect(wrapper.get('#docs-section-title').text()).toBe(docCategories[0]!.sections[0]!.title)
    await input.setValue('')
    expect(wrapper.findAll('button[aria-controls^="docs-category-"]')).toHaveLength(4)
  })

  it('copies the complete original code without language labels or button text', async () => {
    const { wrapper } = await openDocs({ path: '/integration-docs', query: { category: 'image' } })
    const codeBlock = wrapper.get('.docs-code-block')
    const original = codeBlock.get('pre code').element.textContent!
    expect(original).toContain('\n')
    await codeBlock.get('button').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledOnce()
    expect(copyToClipboard).toHaveBeenCalledWith(original)
  })
})
