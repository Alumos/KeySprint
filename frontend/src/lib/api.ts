export const token = () => sessionStorage.getItem('adminToken') || ''

export async function api<T = any>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (!(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  const adminToken = token()
  if (adminToken) headers.set('x-admin-token', adminToken)
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
