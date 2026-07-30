import { beforeEach, describe, expect, it } from 'vitest'
import { consumeVideoStudioImageDraft, saveVideoStudioImageDraft } from '../draft'

describe('video studio image draft', () => {
  beforeEach(() => sessionStorage.clear())

  it('moves an image URL to the video studio once', () => {
    expect(saveVideoStudioImageDraft(' https://cdn.example.com/image.png ')).toBe(true)
    expect(consumeVideoStudioImageDraft()).toMatchObject({
      imageUrl: 'https://cdn.example.com/image.png',
    })
    expect(consumeVideoStudioImageDraft()).toBeNull()
  })
})
