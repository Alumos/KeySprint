import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

// Deployment appearance: the brand shown in the shell and login pages, plus the
// ICP filing number in the footer. Values come from GET /api/site (stored in the
// database by the admin console); the defaults keep a fresh install usable.
export type SiteConfig = {
  title: string
  tagline: string
  icp: string
}

export const DEFAULT_SITE: SiteConfig = {
  title: 'KeySprint',
  tagline: '键跃竞赛管理',
  icp: '',
}

function normalizeSite(raw: Partial<SiteConfig> | null | undefined): SiteConfig {
  const title = (raw?.title || '').trim()
  return {
    title: title || DEFAULT_SITE.title,
    tagline: (raw?.tagline || '').trim(),
    icp: (raw?.icp || '').trim(),
  }
}

type SiteContextValue = { site: SiteConfig; refresh: () => Promise<void> }

const SiteContext = createContext<SiteContextValue>({ site: DEFAULT_SITE, refresh: async () => {} })

export function SiteProvider({ children }: { children: ReactNode }) {
  const [site, setSite] = useState<SiteConfig>(DEFAULT_SITE)

  async function refresh() {
    try {
      // Deliberately raw fetch: this bootstraps the API client, so it must not
      // depend on the authenticated request helper.
      const response = await fetch('/api/site', { cache: 'no-store' })
      if (!response.ok) return
      setSite(normalizeSite((await response.json()) as Partial<SiteConfig>))
    } catch { /* Appearance settings are optional; defaults stay in place. */ }
  }

  useEffect(() => {
    void refresh()
  }, [])

  useEffect(() => {
    document.title = site.title
  }, [site.title])

  return <SiteContext.Provider value={{ site, refresh }}>{children}</SiteContext.Provider>
}

export function useSite() {
  return useContext(SiteContext)
}
