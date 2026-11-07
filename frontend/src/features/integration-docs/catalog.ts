import { marked } from 'marked'
import chatMarkdown from './content/chat.md?raw'
import imageMarkdown from './content/image.md?raw'
import videoMarkdown from './content/video.md?raw'
import speechMarkdown from './content/speech.md?raw'

export interface DocSection {
  id: string
  title: string
  markdown: string
}

export interface DocCategory {
  id: 'chat' | 'image' | 'video' | 'speech'
  title: string
  englishTitle: string
  description: string
  filename: string
  markdown: string
  sections: DocSection[]
}

// Parse tokens rather than lines so headings inside code examples stay intact.
export function splitSections(markdown: string): DocSection[] {
  const sections: DocSection[] = []
  let introduction = ''
  for (const token of marked.lexer(markdown)) {
    if (token.type === 'heading' && token.depth === 1) continue
    if (token.type === 'heading' && token.depth === 2) {
      const slug = token.text.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-').replace(/^-|-$/g, '')
      let id = slug || `section-${sections.length + 1}`
      if (sections.some(section => section.id === id)) id += `-${sections.length + 1}`
      sections.push({ id, title: token.text, markdown: sections.length ? '' : introduction })
    } else if (sections.length) {
      sections[sections.length - 1]!.markdown += token.raw
    } else {
      introduction += token.raw
    }
  }
  return sections.map(section => ({ ...section, markdown: section.markdown.trim() }))
}

const definitions = [
  {
    id: 'chat',
    title: '对话模型',
    englishTitle: 'Chat models',
    description: 'API 接入、CC-Switch 与桌面客户端配置',
    filename: '对话模型接入文档.md',
    markdown: chatMarkdown
  },
  {
    id: 'image',
    title: '图片生成',
    englishTitle: 'Image generation',
    description: '文生图、图片编辑与异步任务',
    filename: '图片生成接入文档.md',
    markdown: imageMarkdown
  },
  {
    id: 'video',
    title: '视频生成',
    englishTitle: 'Video generation',
    description: '统一视频接口与 Muse 调用指南',
    filename: '视频生成接入文档.md',
    markdown: videoMarkdown
  },
  {
    id: 'speech',
    title: '语音合成',
    englishTitle: 'Text to speech',
    description: 'TTS、预设音色与双人对话',
    filename: '语音合成接入文档.md',
    markdown: speechMarkdown
  }
] satisfies Omit<DocCategory, 'sections'>[]

export const docCategories: DocCategory[] = definitions.map(category => ({
  ...category,
  sections: splitSections(category.markdown)
}))

export function resolveSelection(categoryId: unknown, sectionId: unknown) {
  const category = docCategories.find(item => item.id === categoryId) ?? docCategories[0]!
  const section = category.sections.find(item => item.id === sectionId) ?? category.sections[0]!
  return { category, section }
}

export function filterCategories(query: string): DocCategory[] {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (!terms.length) return docCategories
  return docCategories.map(category => ({
    ...category,
    sections: category.sections.filter(section => {
      const text = `${category.title} ${category.englishTitle} ${section.title} ${section.markdown}`.toLowerCase()
      return terms.every(term => text.includes(term))
    })
  })).filter(category => category.sections.length)
}
