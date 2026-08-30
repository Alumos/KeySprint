export const token = () => sessionStorage.getItem('adminToken') || ''

export async function api<T = any>(path: string, init: RequestInit = {}): Promise<T> {
  if (path === '/api/results' && typeof init.body === 'string') {
    try {
      const body = JSON.parse(init.body) as Record<string, unknown>
      const classId = Number(sessionStorage.getItem('studentClassId') || 0)
      if (classId > 0 && !body.classId) body.classId = classId
      init = { ...init, body: JSON.stringify(body) }
    } catch { /* keep the original request body */ }
  }
  const headers = new Headers(init.headers)
  if (!(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  if (token()) headers.set('x-admin-token', token())
  const res = await fetch(path, { ...init, headers, cache: 'no-store' })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401) sessionStorage.removeItem('adminToken')
    throw new Error(data.error || `请求失败 (${res.status})`)
  }
  return data
}

export const json = (method: string, body?: unknown): RequestInit => ({ method, body: body === undefined ? undefined : JSON.stringify(body) })

export function wsURL() { return `${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws` }
