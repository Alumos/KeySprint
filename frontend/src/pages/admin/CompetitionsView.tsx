import { BarChart3, Clipboard, Play, Plus, Square, Trash2 } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { AppSelect, Badge, Button, Empty, Field, Input, Modal, OptionsMenu, Panel, PanelHeader, StatStrip } from '../../components/ui'
import { api, json, wsURL } from '../../lib/api'
import { DEFAULT_TEXT_OPTIONS, countEnabledOptions, enabledOptionLabels, resolveTextOptions } from '../../lib/textOptions'
import type { Article, Competition, Student } from '../../lib/types'

type Confirmation = { type: 'start' | 'stop' | 'delete'; comp: Competition }
type CreateResponse = { id: number; accessCode: string; created: boolean }

function newRequestID() {
  try {
    const values = new Uint32Array(4)
    window.crypto.getRandomValues(values)
    return Array.from(values, value => value.toString(36)).join('-')
  } catch {
    return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
  }
}

// Old rows predate text_options and only carry pinyin_enabled, so every read
// path normalises the payload before counting or labelling it.
function optionsOf(comp: Competition) {
  return resolveTextOptions(comp)
}

export default function CompetitionsView({
  notify,
  openResults,
}: {
  notify: (message: string) => void
  openResults: (id: number) => void
}) {
  const [items, setItems] = useState<Competition[]>([])
  const [articles, setArticles] = useState<Article[]>([])
  const [articleId, setArticleId] = useState('')
  const [minutes, setMinutes] = useState('5')
  const [code, setCode] = useState('')
  const [options, setOptions] = useState(DEFAULT_TEXT_OPTIONS)
  const [participants, setParticipants] = useState<Record<number, Student[]>>({})
  const [showPeople, setShowPeople] = useState<Competition | null>(null)
  const [confirm, setConfirm] = useState<Confirmation | null>(null)
  const [creating, setCreating] = useState(false)
  const [executing, setExecuting] = useState(false)
  const creatingGuard = useRef(false)
  const requestID = useRef(newRequestID())

  const loadCompetitions = useCallback(async () => {
    const competitions = await api<Competition[]>('/api/competitions')
    setItems(competitions)

    // Fetch each active room once, then update state once. Live changes arrive
    // over the socket and no longer trigger an N+1 refresh per submitted score.
    const active = competitions.filter(item => item.status !== 'stopped')
    const counts = await Promise.all(active.map(async item => {
      try {
        const data = await api<{ participants: Student[] }>(`/api/competitions/${item.id}/participants`)
        return [item.id, data.participants] as const
      } catch {
        return [item.id, []] as const
      }
    }))
    setParticipants(Object.fromEntries(counts))
  }, [])

  useEffect(() => {
    void Promise.all([
      loadCompetitions(),
      api<Article[]>('/api/articles/selected').then(setArticles),
    ]).catch(error => notify((error as Error).message))
  }, [loadCompetitions, notify])

  // Editing the form represents a new creation intent. If a response was lost,
  // retrying the unchanged form keeps the same ID and returns the first row.
  useEffect(() => {
    requestID.current = newRequestID()
  }, [articleId, minutes, code, options])

  const watchKey = useMemo(() => items.map(item => item.id).join(','), [items])
  useEffect(() => {
    if (!watchKey) return
    let closed = false
    let reconnectTimer = 0
    let socket: WebSocket | null = null

    // The socket is ticket-authenticated now: every connect attempt fetches a
    // fresh one-time ticket first. A failed ticket fetch stays silent and only
    // retries, so a network blip cannot spam the teacher with toasts.
    async function connect() {
      let ticket = ''
      try {
        const response = await api<{ ticket: string }>('/api/ws-ticket', { method: 'POST' })
        ticket = response.ticket
      } catch {
        if (!closed) reconnectTimer = window.setTimeout(connect, 1800)
        return
      }
      if (closed) return
      const connection = new WebSocket(`${wsURL()}?ticket=${encodeURIComponent(ticket)}`)
      socket = connection
      connection.onopen = () => connection.send(JSON.stringify({
        type: 'teacher:watch',
        competitionIds: watchKey.split(',').map(Number),
      }))
      connection.onmessage = event => {
        const message = JSON.parse(event.data)
        if (message.type === 'participants:update') {
          setParticipants(previous => ({
            ...previous,
            [message.data.competitionId]: message.data.participants,
          }))
        }
        if (message.type === 'competition:stop') void loadCompetitions()
      }
      connection.onclose = () => {
        if (!closed) reconnectTimer = window.setTimeout(connect, 1800)
      }
    }

    void connect()
    return () => {
      closed = true
      window.clearTimeout(reconnectTimer)
      socket?.close()
    }
  }, [loadCompetitions, watchKey])

  async function create() {
    if (creatingGuard.current || !articleId) return
    creatingGuard.current = true
    setCreating(true)
    try {
      const response = await api<CreateResponse>('/api/competition/create', json('POST', {
        articleId: Number(articleId),
        timeLimit: Number(minutes) * 60,
        customCode: code,
        pinyinEnabled: options.pinyin,
        textOptions: options,
        requestId: requestID.current,
      }))
      notify(response.created === false ? '该创建请求已处理，未重复生成竞赛' : `竞赛已创建：${response.accessCode}`)
      requestID.current = newRequestID()
      setCode('')
      await loadCompetitions()
    } catch (error) {
      notify((error as Error).message)
    } finally {
      creatingGuard.current = false
      setCreating(false)
    }
  }

  async function execute() {
    if (!confirm || executing) return
    const { id } = confirm.comp
    setExecuting(true)
    try {
      if (confirm.type === 'start') await api('/api/competition/start', json('POST', { id }))
      if (confirm.type === 'stop') await api('/api/competition/stop', json('POST', { id }))
      if (confirm.type === 'delete') await api(`/api/competitions/${id}`, { method: 'DELETE' })
      notify(confirm.type === 'start' ? '竞赛已开始' : confirm.type === 'stop' ? '竞赛已停止' : '竞赛已删除')
      setConfirm(null)
      await loadCompetitions()
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setExecuting(false)
    }
  }

  const active = items.filter(item => item.status !== 'stopped')
  const running = items.filter(item => item.status === 'running')

  return <div className="admin-page">
    <StatStrip items={[
      { label: '进行中', value: running.length, detail: '正在接受学生输入' },
      { label: '等待开始', value: items.filter(item => item.status === 'waiting').length },
      { label: '历史竞赛', value: items.filter(item => item.status === 'stopped').length },
      { label: '当前在线', value: Object.values(participants).reduce((count, list) => count + list.length, 0), detail: '按比赛累计' },
    ]}/>

    <Panel>
      <PanelHeader title="创建竞赛" description="选择范文并设置比赛限时、编号与输入辅助"/>
      <div className="panel-body create-competition">
        <Field label="竞赛范文">
          <AppSelect value={articleId} onValueChange={setArticleId} placeholder="选择已选用的文章" items={articles.map(article => ({
            value: String(article.id),
            label: article.uploader ? `${article.title} · ${article.uploader}` : article.title,
          }))}/>
        </Field>
        <Field label="限时（分钟）">
          <Input type="number" min="1" max="1440" value={minutes} onChange={event => setMinutes(event.target.value)}/>
        </Field>
        <Field label="比赛编号" hint="留空自动生成">
          <Input className="mono" maxLength={20} value={code} onChange={event => setCode(event.target.value.toUpperCase())} placeholder="自动生成"/>
        </Field>
        <div className="competition-options">
          <OptionsMenu value={options} onChange={setOptions}/>
        </div>
        <Button onClick={create} disabled={!articleId || creating}>
          <Plus size={14}/>{creating ? '正在创建…' : '创建竞赛'}
        </Button>
      </div>
    </Panel>

    <Panel>
      <PanelHeader title="活跃竞赛" description="学生可通过比赛编号进入等待室"/>
      <div className="table-wrap">{active.length ? <table className="data-table">
        <thead><tr><th>文章</th><th>编号</th><th>状态</th><th>文本选项</th><th>限时</th><th>已进入</th><th style={{ textAlign: 'right' }}>操作</th></tr></thead>
        <tbody>{active.map(comp => {
          const optionState = optionsOf(comp)
          const optionCount = countEnabledOptions(optionState)
          return <tr key={comp.id}>
            <td className="td-title">{comp.article_title}</td>
            <td><button className="code-copy mono" onClick={() => navigator.clipboard.writeText(comp.access_code).then(() => notify('比赛编号已复制'))}>{comp.access_code}<Clipboard size={12}/></button></td>
            <td><Badge tone={comp.status === 'running' ? 'success' : 'warning'}>{comp.status === 'running' ? '进行中' : '等待开始'}</Badge></td>
            <td><span title={enabledOptionLabels(optionState).join('、') || '无'}><Badge tone={optionCount > 1 ? 'blue' : 'neutral'}>{optionCount} 项</Badge></span></td>
            <td>{Math.round(comp.time_limit / 60)} 分钟</td>
            <td><button className="link-button" onClick={() => setShowPeople(comp)}>{participants[comp.id]?.length || 0} 人</button></td>
            <td><div className="row-actions">
              {comp.status === 'waiting'
                ? <Button size="sm" onClick={() => setConfirm({ type: 'start', comp })}><Play size={13}/>开始</Button>
                : <Button size="sm" variant="danger" onClick={() => setConfirm({ type: 'stop', comp })}><Square size={13}/>停止</Button>}
              <Button size="icon" variant="ghost" onClick={() => openResults(comp.id)} title="查看成绩"><BarChart3 size={14}/></Button>
            </div></td>
          </tr>
        })}</tbody>
      </table> : <Empty title="没有活跃竞赛" description="在上方创建一场新竞赛"/>}</div>
    </Panel>

    <Panel>
      <PanelHeader title="竞赛历史" description="所有已创建竞赛及其状态"/>
      <div className="table-wrap">{items.length ? <table className="data-table">
        <thead><tr><th>文章</th><th>编号</th><th>状态</th><th>文本选项</th><th>创建时间</th><th style={{ textAlign: 'right' }}>操作</th></tr></thead>
        <tbody>{items.map(comp => {
          const optionState = optionsOf(comp)
          const optionCount = countEnabledOptions(optionState)
          return <tr key={comp.id}>
            <td className="td-title">{comp.article_title}</td><td className="mono">{comp.access_code}</td>
            <td><Badge tone={comp.status === 'running' ? 'success' : comp.status === 'waiting' ? 'warning' : 'neutral'}>{comp.status === 'running' ? '进行中' : comp.status === 'waiting' ? '等待开始' : '已结束'}</Badge></td>
            <td><span title={enabledOptionLabels(optionState).join('、') || '无'}><Badge tone={optionCount > 1 ? 'blue' : 'neutral'}>{optionCount} 项</Badge></span></td><td>{comp.created_at}</td>
            <td><div className="row-actions">
              <Button size="icon" variant="ghost" onClick={() => openResults(comp.id)}><BarChart3 size={14}/></Button>
              <Button size="icon" variant="ghost" disabled={comp.status === 'running'} onClick={() => setConfirm({ type: 'delete', comp })}><Trash2 size={14}/></Button>
            </div></td>
          </tr>
        })}</tbody>
      </table> : <Empty title="暂无竞赛记录"/>}</div>
    </Panel>

    <Modal open={!!showPeople} onOpenChange={value => !value && setShowPeople(null)} title="已进入比赛的学生" description={`${showPeople?.article_title || ''} · 实时更新`}>
      <div className="table-wrap"><table className="data-table"><thead><tr><th>学号</th><th>姓名</th></tr></thead><tbody>
        {(participants[showPeople?.id || 0] || []).map(person => <tr key={person.studentId}><td>{person.studentId}</td><td>{person.studentName}</td></tr>)}
      </tbody></table>{!participants[showPeople?.id || 0]?.length && <Empty title="还没有学生进入"/>}</div>
    </Modal>

    <Modal open={!!confirm} onOpenChange={value => !value && setConfirm(null)} title={confirm?.type === 'start' ? '开始竞赛' : confirm?.type === 'stop' ? '停止竞赛' : '删除竞赛'} description={confirm?.comp.article_title} footer={<>
      <Button variant="outline" onClick={() => setConfirm(null)} disabled={executing}>取消</Button>
      <Button variant={confirm?.type === 'stop' || confirm?.type === 'delete' ? 'danger' : 'default'} onClick={execute} disabled={executing}>{executing ? '处理中…' : '确认'}</Button>
    </>}>
      <p style={{ fontSize: 13 }}>{confirm?.type === 'start' ? '发令后所有等待中的学生会立即开始计时。' : confirm?.type === 'stop' ? '停止后所有学生会立即结束并提交成绩。' : '竞赛及其全部成绩会被永久删除。'}</p>
    </Modal>
  </div>
}
