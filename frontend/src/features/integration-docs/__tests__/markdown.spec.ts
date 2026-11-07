import { describe, expect, it } from 'vitest'
import { splitSections } from '../catalog'
import { renderDocument } from '../markdown'

describe('Integration docs Markdown', () => {
  it('keeps fenced headings inside their example when creating chapter links', () => {
    const sections = splitSections('# API\n\nIntro\n\n## Start\n\n```markdown\n## Example heading\n```\n\n## Finish\n\nDone')
    expect(sections.map(section => section.title)).toEqual(['Start', 'Finish'])
    expect(sections[0]!.markdown).toContain('Intro')
    expect(sections[0]!.markdown).toContain('```markdown\n## Example heading\n```')
    expect(sections[1]!.markdown).toBe('Done')
  })

  it('renders scrollable tables and preserves code as plain text for display and copying', () => {
    const code = 'const value = "<script>alert(1)</script>";\nconsole.log(value)'
    const blocks = renderDocument(`| Field | Description |\n| --- | --- |\n| model | Model name |\n\n\`\`\`javascript\n${code}\n\`\`\``)
    const html = blocks.filter(block => block.type === 'html').map(block => block.html).join('')
    const fragment = document.createElement('div')
    fragment.innerHTML = html
    expect(fragment.querySelector('.docs-table-scroll[tabindex="0"] table')).not.toBeNull()
    expect(fragment.querySelector('td')?.textContent).toBe('model')
    expect(blocks.find(block => block.type === 'code')).toEqual({ type: 'code', language: 'javascript', text: code })
    expect(fragment.querySelector('script')).toBeNull()
  })

  it('removes scripts, event handlers, unsafe links and embedded remote content', () => {
    const blocks = renderDocument([
      '<script>alert(document.cookie)</script>',
      '<img src="https://example.com/tracking.png" onerror="alert(1)">',
      '<iframe src="https://example.com"></iframe>',
      '<form><input autofocus onfocus="alert(1)"><button>Send</button></form>',
      '<p style="color:red" onclick="alert(1)">Readable</p>',
      '<a href="javascript:alert(1)">Unsafe link</a>',
      '[Reference](https://example.com/reference)',
    ].join('\n\n'))
    const fragment = document.createElement('div')
    fragment.innerHTML = blocks.filter(block => block.type === 'html').map(block => block.html).join('')
    expect(fragment.querySelector('script,img,iframe,form,input,button,[onclick],[onerror],[onfocus],[style]')).toBeNull()
    expect(fragment.querySelector('a[href^="javascript:"]')).toBeNull()
    expect(fragment.textContent).toContain('Readable')
    expect(fragment.querySelector('a[href="https://example.com/reference"]')).not.toBeNull()
  })

  it('opens external Markdown and HTML links in a new window without changing local navigation', () => {
    const blocks = renderDocument([
      '[Download](https://example.com/download)',
      '<a href="http://example.org/guide" target="_self">Guide</a>',
      '<a href="//example.net/help">Help</a>',
      '[Local page](/keys)',
      '[Section](#configuration)',
      `[Same origin](${window.location.origin}/integration-docs)`,
    ].join('\n\n'))
    const fragment = document.createElement('div')
    fragment.innerHTML = blocks.filter(block => block.type === 'html').map(block => block.html).join('')
    const links = Array.from(fragment.querySelectorAll('a'))
    for (const link of links.slice(0, 3)) {
      expect(link.target).toBe('_blank')
      expect(link.rel).toBe('noopener noreferrer')
    }
    for (const link of links.slice(3)) {
      expect(link.hasAttribute('target')).toBe(false)
    }
    expect(links).toHaveLength(6)
  })
})
