import { api, json } from './api'

// Word boundaries are a display hint, not a content change: students still type
// the same characters. The jieba dictionary lives in the Go backend, so the
// browser bundle stays small and the server caches identical article texts.
const cache = new Map<string, number[]>()

export async function fetchWordStarts(code: string, text: string): Promise<number[]> {
  const key = `${code}|${text}`
  const cached = cache.get(key)
  if (cached) return cached
  const result = await api<{ starts?: number[] }>('/api/segment', json('POST', { code, text }))
  const starts = Array.isArray(result.starts) ? result.starts : []
  if (cache.size >= 16) cache.clear()
  cache.set(key, starts)
  return starts
}
