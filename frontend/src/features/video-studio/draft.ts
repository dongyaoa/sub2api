const VIDEO_DRAFT_KEY = 'video_studio_image_draft_v1'

export interface VideoStudioImageDraft {
  imageUrl: string
  createdAt: number
}

export function saveVideoStudioImageDraft(imageUrl: string): boolean {
  const normalized = imageUrl.trim()
  if (!normalized || typeof sessionStorage === 'undefined') return false
  try {
    sessionStorage.setItem(VIDEO_DRAFT_KEY, JSON.stringify({
      imageUrl: normalized,
      createdAt: Date.now(),
    } satisfies VideoStudioImageDraft))
    return true
  } catch {
    return false
  }
}

export function consumeVideoStudioImageDraft(): VideoStudioImageDraft | null {
  if (typeof sessionStorage === 'undefined') return null
  try {
    const raw = sessionStorage.getItem(VIDEO_DRAFT_KEY)
    sessionStorage.removeItem(VIDEO_DRAFT_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<VideoStudioImageDraft>
    if (typeof parsed.imageUrl !== 'string' || !parsed.imageUrl.trim()) return null
    return {
      imageUrl: parsed.imageUrl.trim(),
      createdAt: Number(parsed.createdAt) || Date.now(),
    }
  } catch {
    sessionStorage.removeItem(VIDEO_DRAFT_KEY)
    return null
  }
}
