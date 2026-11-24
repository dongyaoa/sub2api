import { i18n } from '@/i18n'

export function studioGenerationErrorMessage(error: unknown, fallback: string): string {
  if (!error || typeof error !== 'object') return fallback
  const { code, type, message } = error as { code?: unknown; type?: unknown; message?: unknown }
  // Auth returns INSUFFICIENT_BALANCE. The gateway's later billing check uses
  // billing_error for several reasons, including this exact balance failure.
  const reason = code ?? type
  if (reason === 'INSUFFICIENT_BALANCE' || (reason === 'billing_error' && message === 'insufficient balance')) {
    return i18n.global.t('mediaStudio.insufficientBalance')
  }
  return fallback
}
