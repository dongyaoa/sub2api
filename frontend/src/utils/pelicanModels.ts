export const DEFAULT_PELICAN_MODEL = 'gpt-6-astra'
export const DEFAULT_CLAUDE_PELICAN_MODEL = 'claude-opus-5-5'
export const DEFAULT_PELICAN_REASONING_EFFORT = 'high'
export const PELICAN_REASONING_EFFORTS = ['low', 'medium', 'high', 'xhigh', 'max'] as const
export type PelicanReasoningEffort = typeof PELICAN_REASONING_EFFORTS[number]

export const PELICAN_MODELS = [
  { value: DEFAULT_PELICAN_MODEL, label: 'GPT-6 Astra' },
  { value: 'gpt-6.1-sol', label: 'GPT-6.1 Sol' },
  { value: 'gpt-6.2-astra', label: 'GPT-6.2 Astra' },
  { value: 'gpt-6.2-sol', label: 'GPT-6.2 Sol' },
] as const

export const CLAUDE_PELICAN_MODELS = [
  { value: DEFAULT_CLAUDE_PELICAN_MODEL, label: 'Claude Opus 5.5' },
  { value: 'claude-sonnet-5-5', label: 'Claude Sonnet 5.5' },
  { value: 'claude-fable-5-2', label: 'Claude Fable 5.2' },
] as const

export function supportsPelicanProtocol(provider: string): boolean {
  return provider === 'openai' || provider === 'anthropic'
}

export function isClaudePelicanModel(model?: string): boolean {
  return !!model && model.length <= 100 && /^claude-[a-z0-9]+(?:[.-][a-z0-9]+)*$/.test(model)
}

export function isOpenAIPelicanModel(model?: string): boolean {
  return PELICAN_MODELS.some(option => option.value === model)
}

export function claudePelicanOptions(models: readonly string[] = []) {
  const options = new Map<string, string>(CLAUDE_PELICAN_MODELS.map(option => [option.value, option.label]))
  for (const model of models) if (isClaudePelicanModel(model) && !options.has(model)) options.set(model, model)
  return Array.from(options, ([value, label]) => ({ value, label }))
}

export function pelicanModelLabel(model?: string): string {
  const value = model || DEFAULT_PELICAN_MODEL
  return PELICAN_MODELS.find(option => option.value === value)?.label || CLAUDE_PELICAN_MODELS.find(option => option.value === value)?.label || value
}

export function pelicanModelClass(model?: string): string {
  return model?.endsWith('-sol')
    ? 'border-sky-200/70 bg-sky-50 text-sky-700 dark:border-sky-500/20 dark:bg-sky-500/10 dark:text-sky-300'
    : 'border-violet-200/70 bg-violet-50 text-violet-700 dark:border-violet-500/20 dark:bg-violet-500/10 dark:text-violet-300'
}
