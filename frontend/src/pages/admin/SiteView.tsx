import { Save } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button, Field, Input, Panel, PanelHeader } from '../../components/ui'
import { api, json } from '../../lib/api'
import { useSite, type SiteConfig } from '../../lib/site'

export default function SiteView({ notify }: { notify: (message: string) => void }) {
  const { site, refresh } = useSite()
  const [form, setForm] = useState<SiteConfig>(site)
  const [busy, setBusy] = useState(false)

  // The provider may finish loading after this view mounts; mirror its value.
  useEffect(() => setForm(site), [site])

  function set(key: keyof SiteConfig, value: string) {
    setForm(current => ({ ...current, [key]: value }))
  }

  async function save() {
    setBusy(true)
    try {
      await api('/api/site', json('PUT', form))
      await refresh()
      notify('系统配置已保存')
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return <div className="admin-page"><Panel>
    <PanelHeader title="系统配置" description="网站外观配置，保存后立即对所有页面生效" />
    <div className="panel-body stack">
      <Field label="网站标题" hint="显示在侧栏、登录页与浏览器标题，最多 40 个字符">
        <Input value={form.title} maxLength={40} onChange={event => set('title', event.target.value)} placeholder="KeySprint" />
      </Field>
      <Field label="网站副标题" hint="显示在侧栏标题下方，可留空，最多 60 个字符">
        <Input value={form.tagline} maxLength={60} onChange={event => set('tagline', event.target.value)} placeholder="键跃竞赛管理" />
      </Field>
      <Field label="ICP 备案号" hint="留空则不显示备案信息；填写后页脚固定跳转 https://beian.miit.gov.cn/">
        <Input value={form.icp} maxLength={40} onChange={event => set('icp', event.target.value)} placeholder="例如：苏ICP备2021038338号-1" />
      </Field>
      <div className="notice notice-success">
        页脚预览：{form.icp ? <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer nofollow">{form.icp}</a> : <span className="muted">未填写备案号，页脚不显示</span>}
      </div>
      <div><Button onClick={save} disabled={busy || !form.title.trim()}><Save size={14} />{busy ? '正在保存...' : '保存配置'}</Button></div>
    </div>
  </Panel></div>
}
