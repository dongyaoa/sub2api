import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))
const modelCardSource = readFileSync(resolve(here, '../ModelPriceCard.vue'), 'utf8')
const modelSquareSource = readFileSync(resolve(here, '../../../views/user/ModelSquareView.vue'), 'utf8')

describe('Model Square dark mode styles', () => {
  it('wraps complete descendant selectors in :global so scoped CSS preserves them', () => {
    expect(modelCardSource).not.toContain(':global(.dark) .')
    expect(modelSquareSource).not.toContain(':global(.dark) .')

    expect(modelCardSource).toContain(':global(.dark .premium-model-card)')
    expect(modelCardSource).toContain(':global(.dark .premium-model-card .metric-box)')
    expect(modelSquareSource).toContain(':global(.dark .premium-filter-panel)')
    expect(modelSquareSource).toContain(':global(.dark .premium-filter-panel .model-square-select .select-trigger)')

    expect(modelCardSource).not.toContain(':global(.dark .metric-box)')
    expect(modelSquareSource).not.toContain(':global(.dark .model-square-select')
  })
})
