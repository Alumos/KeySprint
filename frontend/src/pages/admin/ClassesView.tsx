import { Eye, FileSpreadsheet, RefreshCw, Trash2, Upload } from 'lucide-react'
import { FormEvent, useEffect, useState } from 'react'
import { Badge, Button, Empty, Field, Input, Modal, Panel, PanelHeader, StatStrip } from '../../components/ui'
import { api } from '../../lib/api'
import type { ClassItem, Student } from '../../lib/types'

type ClassSourceStatus = {
  source: 'manual' | 'classorbit'
  configured: boolean
  sync_enabled: boolean
}

export default function ClassesView({ notify }: { notify: (message: string) => void }) {
  const [classes, setClasses] = useState<ClassItem[]>([])
  const [source, setSource] = useState<ClassSourceStatus>({ source: 'manual', configured: false, sync_enabled: false })
  const [name, setName] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [preview, setPreview] = useState<ClassItem | null>(null)
  const [students, setStudents] = useState<Student[]>([])
  const [deleting, setDeleting] = useState<ClassItem | null>(null)

  const load = () => api<ClassItem[]>('/api/classes').then(setClasses)
  useEffect(() => {
    load()
    api<ClassSourceStatus>('/api/classes/integration/status').then(setSource).catch(() => {})
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!file) return
    const form = new FormData()
    form.append('className', name)
    form.append('file', file)
    setBusy(true)
    try {
      const result = await api<{ count: number }>('/api/classes/import', { method: 'POST', body: form })
      notify(`已从 Excel 导入 ${result.count} 名学生`)
      setName('')
      setFile(null)
      load()
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function show(item: ClassItem) {
    setPreview(item)
    setStudents(await api(`/api/classes/${item.id}/students`))
  }

  async function sync() {
    setSyncing(true)
    try {
      const result = await api<{ unchanged: boolean; class_count?: number; student_count?: number }>('/api/classes/sync', { method: 'POST' })
      if (result.unchanged) {
        notify('ClassOrbit 名单没有变化，无需重复写入')
      } else {
        notify(`已从 ClassOrbit 同步 ${result.class_count} 个班级、${result.student_count} 名学生`)
        load()
      }
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setSyncing(false)
    }
  }

  async function remove() {
    if (!deleting) return
    try {
      await api(`/api/classes/${deleting.id}`, { method: 'DELETE' })
      setDeleting(null)
      notify('班级已删除')
      load()
    } catch (error) {
      notify((error as Error).message)
    }
  }

  const sourceLabel = source.source === 'classorbit' ? 'ClassOrbit 接口' : 'Excel 手工导入'
  const sourceDetail = source.source === 'classorbit'
    ? source.configured ? '接口为主要来源，仍可手工导入兜底' : '接口模式尚未配置服务地址'
    : '名单由教师上传并保存在 KeySprint'

  return <div className="admin-page">
    <div className="notice notice-success">
      {source.source === 'classorbit' ? <RefreshCw size={15} /> : <FileSpreadsheet size={15} />}
      当前名单来源：<strong>{sourceLabel}</strong> · {sourceDetail}
    </div>
    <StatStrip items={[
      { label: '班级数量', value: classes.length },
      { label: '学生名额', value: classes.reduce((total, item) => total + item.student_count, 0), detail: '按班级名单累计' },
      { label: '接口班级', value: classes.filter(item => item.source === 'classorbit').length, detail: '由 ClassOrbit 同步' },
      { label: '手工班级', value: classes.filter(item => item.source === 'manual').length, detail: '由 Excel 导入' },
    ]} />
    <div className="admin-grid form-list">
      <Panel>
        <PanelHeader title="导入班级名单" description="手工兜底始终可用；支持 .xlsx 和 .xls，首行需包含学号和姓名" />
        <form className="panel-body stack" onSubmit={submit}>
          <Field label="班级名称"><Input value={name} onChange={event => setName(event.target.value)} placeholder="例如：三年级一班" /></Field>
          <Field label="Excel 文件" hint="自动识别“学号”“姓名”列">
            <label className="file-drop">
              <FileSpreadsheet size={22} />
              <strong>{file?.name || '选择 Excel 文件'}</strong>
              <span>{file ? '点击可重新选择' : '支持 .xlsx/.xls，最大 10MB'}</span>
              <input type="file" accept=".xlsx,.xls" onChange={event => setFile(event.target.files?.[0] || null)} />
            </label>
          </Field>
          <Button disabled={busy || !name.trim() || !file}><Upload size={14} />{busy ? '正在导入...' : '导入名单'}</Button>
        </form>
      </Panel>
      <Panel>
        <PanelHeader
          title="班级列表"
          description="每个班级都会记录自己的名单来源"
          action={source.sync_enabled && <Button variant="outline" size="sm" onClick={sync} disabled={syncing}><RefreshCw size={14} />{syncing ? '正在同步...' : '从 ClassOrbit 同步'}</Button>}
        />
        <div className="table-wrap">
          {classes.length ? <table className="data-table">
            <thead><tr><th>班级</th><th>来源</th><th>学生数</th><th>更新时间</th><th style={{ textAlign: 'right' }}>操作</th></tr></thead>
            <tbody>{classes.map(item => <tr key={item.id}>
              <td className="td-title">{item.name}</td>
              <td>{item.source === 'classorbit' ? <Badge tone="blue">ClassOrbit 接口</Badge> : <Badge>Excel 导入</Badge>}</td>
              <td>{item.student_count} 人</td>
              <td>{item.created_at}</td>
              <td><div className="row-actions">
                <Button variant="ghost" size="icon" onClick={() => show(item)}><Eye size={14} /></Button>
                <Button variant="ghost" size="icon" onClick={() => setDeleting(item)}><Trash2 size={14} /></Button>
              </div></td>
            </tr>)}</tbody>
          </table> : <Empty title="还没有班级" description={source.sync_enabled ? '点击右上角同步，或从左侧导入 Excel' : '从左侧导入第一份 Excel 名单'} />}
        </div>
      </Panel>
    </div>
    <Modal open={!!preview} onOpenChange={open => !open && setPreview(null)} title={`${preview?.name || ''} · 学生名单`} description={`共 ${students.length} 人`}>
      <div className="table-wrap" style={{ maxHeight: '55vh' }}><table className="data-table">
        <thead><tr><th>学号</th><th>姓名</th></tr></thead>
        <tbody>{students.map((student, index) => <tr key={index}><td>{student.student_id}</td><td>{student.student_name}</td></tr>)}</tbody>
      </table></div>
    </Modal>
    <Modal open={!!deleting} onOpenChange={open => !open && setDeleting(null)} title="删除班级" description="学生名单会一并删除，已有成绩不受影响" footer={<><Button variant="outline" onClick={() => setDeleting(null)}>取消</Button><Button variant="danger" onClick={remove}>删除</Button></>}>
      <p style={{ fontSize: 13 }}>确定删除「{deleting?.name}」吗？</p>
    </Modal>
  </div>
}
