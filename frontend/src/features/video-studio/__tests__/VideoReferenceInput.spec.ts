import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import VideoReferenceInput from '../VideoReferenceInput.vue'
import type { VideoReferenceInputValue } from '../types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function mountInput(mediaType: 'image' | 'video', max: number, modelValue: VideoReferenceInputValue[] = []) {
  return mount(VideoReferenceInput, { props: { id: 'reference', label: 'Reference', mediaType, max, modelValue } })
}

async function upload(wrapper: ReturnType<typeof mountInput>, files: File[]) {
  const input = wrapper.find('input[type="file"]')
  Object.defineProperty(input.element, 'files', { value: files, configurable: true })
  await input.trigger('change')
}

describe('video reference input', () => {
  it.each([['image', 10], ['video', 3]] as const)('enforces the %s reference count before accepting files', async (type, max) => {
    const wrapper = mountInput(type, max, Array.from({ length: max - 1 }, () => ({ url: 'https://example.com/media' })))
    try {
      await upload(wrapper, [new File(['a'], 'a', { type: type + (type === 'image' ? '/png' : '/mp4') }), new File(['b'], 'b', { type: type + (type === 'image' ? '/png' : '/mp4') })])
      expect(wrapper.emitted('update:modelValue')).toBeUndefined()
      expect(wrapper.find('[role="alert"]').text()).toBe('videoStudio.referenceCountError')
    } finally { wrapper.unmount() }
  })

  it('rejects oversize or unsupported media and accepts a valid video upload', async () => {
    const wrapper = mountInput('video', 3)
    try {
      const large = new File(['video'], 'large.mp4', { type: 'video/mp4' })
      Object.defineProperty(large, 'size', { value: 20 * 1024 * 1024 + 1 })
      await upload(wrapper, [large])
      expect(wrapper.find('[role="alert"]').text()).toBe('videoStudio.referenceSizeError')
      await upload(wrapper, [new File(['x'], 'invalid.txt', { type: 'text/plain' })])
      expect(wrapper.find('[role="alert"]').text()).toBe('videoStudio.referenceVideoTypeError')
      const video = new File(['video'], 'motion.mp4', { type: 'video/mp4' })
      await upload(wrapper, [video])
      expect(wrapper.emitted('update:modelValue')).toEqual([[[{ file: video, url: '' }]]])
    } finally { wrapper.unmount() }
  })

  it('validates URL schemes and media types before adding a reference', async () => {
    const wrapper = mountInput('image', 1)
    try {
      for (const url of ['javascript:alert(1)', 'file:///private.png', 'data:video/mp4;base64,AAAA']) {
        await wrapper.find('#reference').setValue(url)
        await wrapper.find('#reference').trigger('keydown', { key: 'Enter' })
        expect(wrapper.find('[role="alert"]').text()).toBe('videoStudio.referenceUrlError')
      }
      expect(wrapper.emitted('update:modelValue')).toBeUndefined()
      await wrapper.find('#reference').setValue('https://example.com/image.png')
      await wrapper.find('#reference').trigger('keydown', { key: 'Enter' })
      expect(wrapper.emitted('update:modelValue')).toEqual([[[{ url: 'https://example.com/image.png' }]]])
    } finally { wrapper.unmount() }
  })
})
