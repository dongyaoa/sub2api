import DOMPurify from 'dompurify'
import { Marked, Renderer, type Token, type Tokens } from 'marked'

export type DocumentBlock = { type: 'html'; html: string } | { type: 'code'; language: string; text: string }

const renderer = new Renderer()
renderer.table = function (token: Tokens.Table) {
  return `<div class="docs-table-scroll" tabindex="0">${Renderer.prototype.table.call(this, token)}</div>`
}
const parser = new Marked({ renderer, gfm: true, breaks: false })

export function renderDocument(markdown: string): DocumentBlock[] {
  const blocks: DocumentBlock[] = []
  let pending: Token[] = []
  const flush = () => {
    if (!pending.length) return
    const fragment = DOMPurify.sanitize(parser.parser(pending), {
      USE_PROFILES: { html: true },
      FORBID_TAGS: ['img', 'video', 'audio', 'iframe', 'form', 'input', 'button', 'style'],
      FORBID_ATTR: ['style'],
      RETURN_DOM_FRAGMENT: true
    })
    const content = document.createElement('div')
    content.appendChild(fragment)
    // Apply to sanitized Markdown and HTML links alike; keep local navigation in place.
    for (const link of content.querySelectorAll<HTMLAnchorElement>('a[href]')) {
      try {
        const url = new URL(link.getAttribute('href') || '', window.location.href)
        if (['http:', 'https:'].includes(url.protocol) && url.origin !== window.location.origin) {
          link.target = '_blank'
          link.rel = 'noopener noreferrer'
        }
      } catch {
        // An invalid URL is not an external navigation target.
      }
    }
    blocks.push({ type: 'html', html: content.innerHTML })
    pending = []
  }
  for (const token of parser.lexer(markdown)) {
    if (token.type === 'code') {
      flush()
      blocks.push({ type: 'code', language: token.lang?.split(/\s/)[0] || 'text', text: token.text })
    } else {
      pending.push(token)
    }
  }
  flush()
  return blocks
}
