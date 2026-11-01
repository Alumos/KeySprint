import { Save, Sparkles } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { AppSelect, Badge, Button, Field, Input, Panel, PanelHeader } from '../../components/ui'
import { api, json } from '../../lib/api'
import { clampUIScale, MAX_UI_SCALE, MIN_UI_SCALE, useSite, type SiteConfig } from '../../lib/site'

type AIConfig = { baseUrl: string; model: string; reasoningEffort: string; hasKey: boolean; keyHint: string; models: string[]; modelsBase: string; modelsUpdatedAt: string }

const REASONING_ITEMS = [
  { value: 'low', label: '低（更快）' },
  { value: 'medium', label: '中（默认）' },
  { value: 'high', label: '高（更慢更稳）' },
]

export default function SiteView({ notify }: { notify: (message: string) => void }) {
  const { site, refresh } = useSite()
  const [form, setForm] = useState<SiteConfig>(site)
  const [busy, setBusy] = useState(false)

  const [ai, setAI] = useState<AIConfig>({ baseUrl: '', model: 'gpt-6-luna', reasoningEffort: 'medium', hasKey: false, keyHint: '', models: [], modelsBase: '', modelsUpdatedAt: '' })
  const [apiKey, setApiKey] = useState('')
  const [models, setModels] = useState<string[]>([])
  const [customModel, setCustomModel] = useState(false)
  const [modelText, setModelText] = useState('')
  const [aiBusy, setAIBusy] = useState(false)
  const [probing, setProbing] = useState(false)

  // Probed models plus the saved one, then an explicit escape hatch for
  // gateways whose model names the probe endpoint does not report.
  const modelItems = useMemo(() => {
    const names = [...new Set([ai.model, ...models].filter(Boolean))]
    return [...names.map(name => ({ value: name, label: name })), { value: '__custom', label: '自定义模型…' }]
  }, [ai.model, models])

  // The provider may finish loading after this view mounts; mirror its value.
  useEffect(() => setForm(site), [site])
  useEffect(() => {
    api<AIConfig>('/api/ai/config')
      .then(config => {
        setAI(config)
        // The last probed list is cached server-side; reuse it on every load.
        setModels(config.models || [])
      })
      .catch(() => {})
  }, [])

  function set(key: keyof SiteConfig, value: string) {
    setForm(current => ({ ...current, [key]: value }))
  }
  function patchAI(key: 'baseUrl' | 'model' | 'reasoningEffort', value: string) {
    setAI(current => ({ ...current, [key]: value }))
  }

  async function save() {
    setBusy(true)
    try {
      await api('/api/site', json('PUT', { ...form, ui_scale: clampUIScale(form.ui_scale) }))
      await refresh()
      notify('系统配置已保存')
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function saveAI() {
    const model = (customModel ? modelText : ai.model).trim()
    if (!model) {
      notify('请先选择或填写模型')
      return
    }
    setAIBusy(true)
    try {
      await api('/api/ai/config', json('PUT', { baseUrl: ai.baseUrl, apiKey, model, reasoningEffort: ai.reasoningEffort }))
      setApiKey('')
      setCustomModel(false)
      notify('AI 配置已保存')
      const refreshed = await api<AIConfig>('/api/ai/config')
      setAI(refreshed)
      setModels(refreshed.models || [])
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setAIBusy(false)
    }
  }

  async function probe() {
    if (!ai.baseUrl.trim()) {
      notify('请先填写 AI 接口地址')
      return
    }
    setProbing(true)
    try {
      const result = await api<{ models: string[]; count: number; base: string; updatedAt: string }>('/api/ai/models', { method: 'POST' })
      setModels(result.models)
      setAI(current => ({ ...current, models: result.models, modelsBase: result.base, modelsUpdatedAt: result.updatedAt }))
      if (result.models.length === 1 && !customModel) patchAI('model', result.models[0])
      notify(`上游提供 ${result.count} 个模型，已缓存（下次探测会覆盖）`)
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setProbing(false)
    }
  }

  return <div className="admin-page">
    <Panel>
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
        <Field label="界面字号" hint={`预设 100%；可调 ${MIN_UI_SCALE}–${MAX_UI_SCALE}，保存后所有页面按百分比缩放（当前 ${form.ui_scale || 100}%）。手机窄屏（≤760px）自动按 100% 显示，避免横向滚动`}>
          <Input type="number" min={MIN_UI_SCALE} max={MAX_UI_SCALE} step={5} value={form.ui_scale}
            onChange={event => setForm(current => ({ ...current, ui_scale: Number(event.target.value) }))}
            onBlur={() => setForm(current => ({ ...current, ui_scale: clampUIScale(current.ui_scale) }))} />
        </Field>
        <div className="notice notice-success">
          页脚预览：{form.icp ? <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer nofollow">{form.icp}</a> : <span className="muted">未填写备案号，页脚不显示</span>}
        </div>
        <div><Button onClick={save} disabled={busy || !form.title.trim()}><Save size={14} />{busy ? '正在保存...' : '保存配置'}</Button></div>
      </div>
    </Panel>
    <Panel>
      <PanelHeader title="AI 文章生成" description="对接 OpenAI 兼容接口；保存后，文章管理里的「AI 生成」即可使用" action={<Badge tone="blue"><Sparkles size={11} />{ai.model || '未选择模型'}</Badge>} />
      <div className="panel-body stack">
        <Field label="AI 接口地址" hint="OpenAI 兼容的 /v1 地址，例如 https://aiko.8041024.xyz/v1">
          <Input value={ai.baseUrl} onChange={event => patchAI('baseUrl', event.target.value)} placeholder="https://your-gateway/v1" />
        </Field>
        <Field label="API 密钥" hint={ai.hasKey ? `已保存（${ai.keyHint}），留空表示不修改` : '只保存在本机数据库，不会返回给页面'}>
          <Input type="password" value={apiKey} onChange={event => setApiKey(event.target.value)} placeholder={ai.hasKey ? '留空不修改' : '输入 API Key'} autoComplete="new-password" />
        </Field>
        <Field label="模型" hint={models.length
          ? `已缓存 ${models.length} 个模型${ai.modelsUpdatedAt ? `（${ai.modelsUpdatedAt} 探测）` : ''}${ai.modelsBase && ai.modelsBase !== ai.baseUrl ? `，来自 ${ai.modelsBase}` : ''}；点「探测模型」可刷新`
          : '点击「探测模型」拉取上游列表后从下拉选择；不在列表里的模型可选「自定义模型…」手动填写'}>
          <div style={{ display: 'flex', gap: 8, alignItems: 'flex-start' }}>
            <div style={{ flex: 1 }}>
              <AppSelect value={customModel ? '__custom' : ai.model} placeholder="选择或探测模型" items={modelItems}
                onValueChange={value => {
                  if (value === '__custom') {
                    setModelText(ai.model)
                    setCustomModel(true)
                    return
                  }
                  setCustomModel(false)
                  patchAI('model', value)
                }} />
            </div>
            <Button variant="outline" onClick={probe} disabled={probing || !ai.baseUrl.trim()}>{probing ? '探测中...' : '探测模型'}</Button>
          </div>
          {customModel && <Input value={modelText} onChange={event => setModelText(event.target.value)} placeholder="输入上游模型名，例如 gpt-6-luna" style={{ marginTop: 8 }} />}
        </Field>
        <Field label="思考强度" hint="推理型模型可调；上游不支持该参数时会自动降级重试">
          <AppSelect value={ai.reasoningEffort} onValueChange={value => patchAI('reasoningEffort', value)} items={REASONING_ITEMS} />
        </Field>
        <div><Button onClick={saveAI} disabled={aiBusy}><Save size={14} />{aiBusy ? '正在保存...' : '保存 AI 配置'}</Button></div>
      </div>
    </Panel>
  </div>
}
