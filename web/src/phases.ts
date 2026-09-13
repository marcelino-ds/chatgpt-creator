import type { WorkerPhase } from './types'

/** Maps a backend step label onto a coarse worker phase. */
export function phaseOf(step: string, statusCode: number): WorkerPhase {
  const s = step.toLowerCase()
  if (statusCode >= 400) return 'error'
  if (s.includes('visit homepage')) return 'warmup'
  if (s.includes('csrf') || s.includes('signin') || s.includes('authorize')) return 'auth'
  if (s.includes('register')) return 'register'
  if (s.includes('otp')) return 'otp'
  if (s.includes('create account')) return 'account'
  if (s.includes('callback') || s.includes('verify session')) return 'callback'
  return 'idle'
}

export const PHASE_ORDER: WorkerPhase[] = [
  'warmup',
  'auth',
  'register',
  'otp',
  'account',
  'callback',
]

export const PHASE_LABEL: Record<WorkerPhase, string> = {
  idle: 'idle',
  warmup: 'warmup',
  auth: 'auth',
  register: 'register',
  otp: 'otp',
  account: 'account',
  callback: 'session',
  done: 'done',
  error: 'error',
}

export function fmtDuration(ms: number): string {
  if (!ms || ms < 0) return '0s'
  const total = Math.floor(ms / 1000)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

export function fmtClock(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '--:--:--'
  return d.toLocaleTimeString([], { hour12: false })
}

/** Renders a unicode meter, e.g. ████████░░░░░░░░ */
export function meter(ratio: number, width = 24): string {
  const clamped = Math.max(0, Math.min(1, ratio))
  const filled = Math.round(clamped * width)
  return '█'.repeat(filled) + '░'.repeat(width - filled)
}
