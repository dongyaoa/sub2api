import { retainedPelicanArtworks } from '@/utils/pelicanArtworks'
import type { PelicanMonitorGroup, PelicanRun } from '@/api/pelicanMonitor'
export type PelicanViewMode = 'single' | 'paired' | 'stacked'
export interface PelicanGalleryModel {
  plan: PelicanMonitorGroup
  name: string
  badge: string
  dot: string
  works: PelicanRun[]
  auto: Set<number>
  completed: number
}
const modelDots = ['var(--pelican-model-primary)', '#0ea5e9', '#8b5cf6', '#f59e0b']
export const pelicanModelDot = (index: number) => modelDots[index % modelDots.length] ?? modelDots[0]
export function pelicanEffortLabel(effort?: string) {
  if (!effort) return '—'
  return effort === 'xhigh' ? 'XHigh' : effort.charAt(0).toUpperCase() + effort.slice(1)
}
export const pelicanActive = (run?: PelicanRun | null) => run?.status === 'running' || run?.status === 'pending'
export function pelicanWorks(group: PelicanMonitorGroup) {
  const latest = group.latest_run
  const completed = retainedPelicanArtworks(group)
  return latest && pelicanActive(latest) ? [latest, ...completed] : completed
}
export function pelicanInterval(seconds: number): { minutes?: number; seconds?: number } { return seconds % 60 === 0 ? { minutes: seconds / 60 } : { seconds } }
export function pelicanDuration(run: PelicanRun) {
  const value = run.duration_ms
  if (value == null || !Number.isFinite(value) || value < 0) return '—'
  const seconds = Math.round(value / 1000)
  return seconds >= 60 ? `${Math.floor(seconds / 60)}m ${seconds % 60}s` : `${(value / 1000).toFixed(1)}s`
}
const dateFormatters = new Map<string, Intl.DateTimeFormat>()
export function pelicanDate(value?: string | null, locale = 'zh-CN', includeYear = false) {
  if (!value) return '—'
  const time = new Date(value)
  if (!Number.isFinite(time.getTime())) return '—'
  const key = `${locale}:${includeYear}`
  let formatter = dateFormatters.get(key)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat(locale, { year: includeYear ? 'numeric' : undefined, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
    if (dateFormatters.size >= 8) dateFormatters.delete(dateFormatters.keys().next().value!)
    dateFormatters.set(key, formatter)
  }
  return formatter.format(time)
}
export function pelicanCountdown(value: string | null, now: number) {
  const remaining = value ? Math.ceil((Date.parse(value) - now) / 1000) : 0
  if (!Number.isFinite(remaining) || remaining <= 0) return null
  const minutes = Math.floor(remaining / 60), seconds = remaining % 60
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`
}
export function pelicanTime(stamp: string) {
  const date = new Date(stamp)
  if (!Number.isFinite(date.getTime())) return '—'
  return [date.getHours(), date.getMinutes(), date.getSeconds()].map(value => String(value).padStart(2, '0')).join(':')
}
