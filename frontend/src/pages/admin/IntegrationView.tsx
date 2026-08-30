import { Save, Settings2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { AppSelect, Button, Field, Input, Panel, PanelHeader } from '../../components/ui'
import { api, json } from '../../lib/api'

type Config = {
  source: 'manual' | 'classorbit'
  url: string
  path: string
  token: string
  timeout: string
}

type ConnectionResult = { class_count: number; student_count: number }

export default function IntegrationView({ notify }: { notify: (message: string) => void }) {
  const [config, setConfig] = useState<Config>({
    source: 'manual',
    url: '',
    path: '/api/integration/classes',
    token: '',
    timeout: '8s',
  })
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api<Config>('/api/integration/class-system').then(setConfig).catch(error => notify((error as Error).message))
  }, [])

  function set(key: keyof Config, value: string) {
    setConfig(current => ({ ...current, [key]: value }))
  }

  async function save() {
    setBusy(true)
    let saved = false
    try {
      await api('/api/integration/class-system', json('PUT', config))
      saved = true
      if (config.source === 'classorbit') {
        const result = await api<ConnectionResult>('/api/integration/class-system/test', { method: 'POST' })
        notify(`配置已保存，连接成功：${result.class_count} 个班级、${result.student_count} 名学生`)
      } else {
        notify('班级名单来源配置已保存')
      }
    } catch (error) {
      const message = (error as Error).message
      notify(saved ? `配置已保存，但连接测试失败：${message}` : message)
    } finally {
      setBusy(false)
    }
  }

  const remote = config.source === 'classorbit'
  return <div className="admin-page"><Panel>
    <PanelHeader title="班级名单来源" description="选择主要来源；Excel 手工导入始终保留为课堂兜底" />
    <div className="panel-body stack">
      <div className="notice notice-success">
        <Settings2 size={15} />
        {remote ? 'ClassOrbit 接口是主要来源，教师可在班级管理中主动同步。' : '教师自行上传 Excel，名单直接保存在 KeySprint。'}
      </div>
      <Field label="主要来源">
        <AppSelect
          value={config.source}
          onValueChange={value => set('source', value)}
          items={[
            { value: 'manual', label: 'Excel 手工导入' },
            { value: 'classorbit', label: 'ClassOrbit 接口' },
          ]}
        />
      </Field>
      <Field label="ClassOrbit 服务地址" hint={remote ? '启用接口同步时必填' : '手工模式不会请求该地址'}>
        <Input value={config.url} onChange={event => set('url', event.target.value)} placeholder="https://classorbit.example.com" />
      </Field>
      <Field label="班级接口路径"><Input value={config.path} onChange={event => set('path', event.target.value)} placeholder="/api/integration/classes" /></Field>
      <Field label="共享 Bearer 密钥" hint="不会发送教师密码">
        <Input type="password" value={config.token} onChange={event => set('token', event.target.value)} placeholder="CLASS_SYSTEM_TOKEN" autoComplete="new-password" />
      </Field>
      <Field label="请求超时"><Input value={config.timeout} onChange={event => set('timeout', event.target.value)} placeholder="8s" /></Field>
      <div><Button onClick={save} disabled={busy}><Save size={14} />{busy ? '正在保存并检查...' : remote ? '保存并测试连接' : '保存配置'}</Button></div>
    </div>
  </Panel></div>
}
