import type { Account, Job, Snapshot, StartJobRequest } from './types'

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text.trim() || `${res.status} ${res.statusText}`)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  startJob: (body: StartJobRequest) =>
    req<{ job_id: string }>('/api/jobs', {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  pause: () => req<{ ok: boolean }>('/api/jobs/current/pause', { method: 'POST' }),
  resume: () => req<{ ok: boolean }>('/api/jobs/current/resume', { method: 'POST' }),
  stop: () => req<{ ok: boolean }>('/api/jobs/current/stop', { method: 'POST' }),

  snapshot: () => req<Snapshot>('/api/status'),
  jobs: (limit = 50) => req<Job[] | null>(`/api/jobs?limit=${limit}`),
  accounts: (limit = 200, jobId?: string) =>
    req<Account[] | null>(
      `/api/accounts?limit=${limit}${jobId ? `&job_id=${encodeURIComponent(jobId)}` : ''}`,
    ),
}
