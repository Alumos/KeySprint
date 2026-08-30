import { Save, Settings2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button, Field, Input, Panel, PanelHeader } from '../../components/ui'
import { api, json } from '../../lib/api'

type Config = { url: string; path: string; token: string; timeout: string }

export default function IntegrationView({notify}:{notify:(message:string)=>void}) {
  const [config, setConfig] = useState<Config>({url:'', path:'/api/integration/classes', token:'', timeout:'8s'})
  const [busy, setBusy] = useState(false)
  useEffect(() => { api<Config>('/api/integration/class-system').then(setConfig).catch(e => notify((e as Error).message)) }, [])
  function set(key:keyof Config, value:string) { setConfig(current => ({...current, [key]:value})) }
  async function save() {
    setBusy(true)
    try { await api('/api/integration/class-system', json('PUT', config)); notify('积分系统配置已保存') }
    catch (e) { notify((e as Error).message) }
    finally { setBusy(false) }
  }
  return <div className="admin-page"><Panel>
    <PanelHeader title="班级积分系统" description="配置保存到 KeySprint 数据库，修改后立即生效"/>
    <div className="panel-body stack">
      <div className="notice notice-success"><Settings2 size={15}/>仅管理员可查看和修改共享密钥。教师同步时会按当前登录用户名获取对应班级。</div>
      <Field label="服务地址"><Input value={config.url} onChange={e=>set('url',e.target.value)} placeholder="https://积分系统地址"/></Field>
      <Field label="班级接口路径"><Input value={config.path} onChange={e=>set('path',e.target.value)} placeholder="/api/integration/classes"/></Field>
      <Field label="共享 Bearer 密钥" hint="不会发送教师密码"><Input type="password" value={config.token} onChange={e=>set('token',e.target.value)} placeholder="CLASS_SYSTEM_TOKEN" autoComplete="new-password"/></Field>
      <Field label="请求超时"><Input value={config.timeout} onChange={e=>set('timeout',e.target.value)} placeholder="8s"/></Field>
      <div><Button onClick={save} disabled={busy}><Save size={14}/>{busy?'正在保存...':'保存配置'}</Button></div>
    </div>
  </Panel></div>
}
