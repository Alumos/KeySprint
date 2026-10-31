import { Eye, FileText, Pencil, Plus, Sparkles, Trash2, Upload } from 'lucide-react'
import { ChangeEvent, useEffect, useMemo, useState } from 'react'
import { AppSelect, Badge, Button, Empty, Field, Input, Modal, Pagination, Panel, PanelHeader, StatStrip } from '../../components/ui'
import { api, json } from '../../lib/api'
import type { Article } from '../../lib/types'

const PAGE_SIZE = 10
const STYLE_ITEMS = ['记叙文', '说明文', '散文', '童话', '科普'].map(value => ({ value, label: value }))
type SortKey = 'newest' | 'oldest' | 'chars-desc' | 'chars-asc'

function charsOf(text: string) { return Array.from(text.replace(/\s/g, '')).length }
function isAI(article: Article) { return article.source === 'ai' }

// One table for the whole library: filters by uploader/source/sort replace the
// old "my articles + shared library" split, so the page stays short and every
// article (manual or AI generated) is managed in the same place.
export default function ArticlesView({ notify }: { notify: (message: string) => void }) {
  const [library, setLibrary] = useState<Article[]>([])
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [editing, setEditing] = useState<Article | null>(null)
  const [preview, setPreview] = useState<Article | null>(null)
  const [confirm, setConfirm] = useState<Article | null>(null)
  const [busy, setBusy] = useState(false)
  const [query, setQuery] = useState('')
  const [uploader, setUploader] = useState('all')
  const [source, setSource] = useState('all')
  const [sort, setSort] = useState<SortKey>('newest')
  const [page, setPage] = useState(1)
  const [aiOpen, setAiOpen] = useState(false)
  const [aiTopic, setAiTopic] = useState('')
  const [aiStyle, setAiStyle] = useState('记叙文')
  const [aiMin, setAiMin] = useState('300')
  const [aiMax, setAiMax] = useState('600')
  const [aiExtra, setAiExtra] = useState('')
  const [generating, setGenerating] = useState(false)

  async function load() {
    try {
      setLibrary(await api<Article[]>('/api/library'))
    } catch (error) {
      notify((error as Error).message)
    }
  }
  useEffect(() => { load() }, [])
  useEffect(() => { setPage(1) }, [query, uploader, source, sort])

  const teachers = useMemo(
    () => Array.from(new Set(library.map(item => item.uploader || '').filter(Boolean))).sort((a, b) => a.localeCompare(b, 'zh')),
    [library],
  )
  const filtered = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const list = library.filter(item => {
      if (uploader === 'me' && !item.mine) return false
      if (uploader !== 'all' && uploader !== 'me' && (item.uploader || '') !== uploader) return false
      if (source !== 'all' && (item.source || 'manual') !== source) return false
      if (keyword && !item.title.toLowerCase().includes(keyword)) return false
      return true
    })
    return [...list].sort((a, b) => {
      if (sort === 'chars-desc') return charsOf(b.content) - charsOf(a.content)
      if (sort === 'chars-asc') return charsOf(a.content) - charsOf(b.content)
      const byDate = a.created_at.localeCompare(b.created_at)
      return sort === 'oldest' ? byDate : -byDate
    })
  }, [library, query, uploader, source, sort])
  const pages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const safePage = Math.min(page, pages)
  const paged = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)
  const mine = library.filter(item => item.mine).length
  const aiCount = library.filter(isAI).length
  const picked = library.filter(item => item.mine || item.picked).length

  function file(event: ChangeEvent<HTMLInputElement>) {
    const input = event.target.files?.[0]
    if (!input) return
    setTitle(input.name.replace(/\.txt$/i, ''))
    input.text().then(setContent)
  }
  async function save() {
    if (!title.trim() || !content.trim()) return
    setBusy(true)
    try {
      if (editing) await api(`/api/articles/${editing.id}`, json('PUT', { title, content }))
      else await api('/api/articles', json('POST', { title, content }))
      notify(editing ? '文章已更新' : '文章已添加')
      setTitle('')
      setContent('')
      setEditing(null)
      await load()
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setBusy(false)
    }
  }
  function edit(article: Article) {
    setEditing(article)
    setTitle(article.title)
    setContent(article.content)
    scrollTo({ top: 0, behavior: 'smooth' })
  }
  function cancelEdit() {
    setEditing(null)
    setTitle('')
    setContent('')
  }
  async function remove() {
    if (!confirm) return
    try {
      await api(`/api/articles/${confirm.id}`, { method: 'DELETE' })
      setConfirm(null)
      notify('文章已删除')
      load()
    } catch (error) {
      notify((error as Error).message)
    }
  }
  async function pick(article: Article, value: boolean) {
    try {
      await api('/api/library/pick', json('POST', { articleId: article.id, picked: value }))
      notify(value ? '已加入我的选择' : '已移出我的选择')
      load()
    } catch (error) {
      notify((error as Error).message)
    }
  }
  async function generate() {
    if (!aiTopic.trim() || generating) return
    setGenerating(true)
    try {
      const result = await api<{ title: string; chars: number }>('/api/ai/generate', json('POST', {
        topic: aiTopic,
        style: aiStyle,
        minChars: Number(aiMin) || 300,
        maxChars: Number(aiMax) || 600,
        extra: aiExtra,
      }))
      notify(`已生成《${result.title}》（${result.chars} 字）并加入文章库`)
      setAiOpen(false)
      setAiTopic('')
      setAiExtra('')
      await load()
    } catch (error) {
      notify((error as Error).message)
    } finally {
      setGenerating(false)
    }
  }

  return <div className="admin-page">
    <StatStrip items={[
      { label: '文章库', value: library.length, detail: '全体教师可见' },
      { label: '我的文章', value: mine, detail: '由当前教师上传' },
      { label: 'AI 生成', value: aiCount, detail: '模型一键生成' },
      { label: '已选用', value: picked, detail: '可用于创建竞赛' },
    ]}/>
    <Panel>
      <PanelHeader title="文章库" description="按上传教师、文章来源筛选，按导入时间或字数排序" action={<Button size="sm" onClick={() => setAiOpen(true)}><Sparkles size={14}/>AI 生成</Button>}/>
      <div className="panel-body filters-panel">
        <Field label="搜索标题"><Input value={query} onChange={event => setQuery(event.target.value)} placeholder="按标题搜索"/></Field>
        <Field label="上传教师"><AppSelect value={uploader} onValueChange={setUploader} items={[{ value: 'all', label: '全部教师' }, { value: 'me', label: '我上传的' }, ...teachers.map(name => ({ value: name, label: name }))]}/></Field>
        <Field label="文章来源"><AppSelect value={source} onValueChange={setSource} items={[{ value: 'all', label: '全部来源' }, { value: 'manual', label: '手动上传' }, { value: 'ai', label: 'AI 生成' }]}/></Field>
        <Field label="排序"><AppSelect value={sort} onValueChange={value => setSort(value as SortKey)} items={[
          { value: 'newest', label: '导入时间：新→旧' },
          { value: 'oldest', label: '导入时间：旧→新' },
          { value: 'chars-desc', label: '字数：多→少' },
          { value: 'chars-asc', label: '字数：少→多' },
        ]}/></Field>
      </div>
      <div className="table-wrap">{filtered.length ? <table className="data-table">
        <thead><tr><th style={{ width: 70 }}>选用</th><th>标题</th><th>字数</th><th>上传教师</th><th>导入时间</th><th style={{ textAlign: 'right' }}>操作</th></tr></thead>
        <tbody>{paged.map(article => <tr key={article.id}>
          <td>{article.mine ? <Badge>我的</Badge> : <input type="checkbox" checked={!!article.picked} onChange={event => pick(article, event.target.checked)} title="加入我的选用"/>}</td>
          <td className="td-title"><FileText size={13} style={{ verticalAlign: -2, marginRight: 7 }}/>{article.title}{isAI(article) && <Badge tone="blue">AI</Badge>}</td>
          <td>{charsOf(article.content)}</td>
          <td>{article.mine ? '我' : article.uploader || '-'}</td>
          <td>{article.created_at}</td>
          <td><div className="row-actions">
            <Button size="icon" variant="ghost" title="预览" onClick={() => setPreview(article)}><Eye size={14}/></Button>
            {!!article.mine && <>
              <Button size="icon" variant="ghost" title="编辑" onClick={() => edit(article)}><Pencil size={14}/></Button>
              <Button size="icon" variant="ghost" title="删除" onClick={() => setConfirm(article)}><Trash2 size={14}/></Button>
            </>}
          </div></td>
        </tr>)}</tbody>
      </table> : <Empty title={library.length ? '没有匹配的文章' : '还没有文章'} description={library.length ? '调整筛选条件试试' : '先手动添加，或用 AI 生成一篇'}/>}</div>
      <Pagination page={safePage} pageSize={PAGE_SIZE} total={filtered.length} onPageChange={setPage}/>
    </Panel>
    <Panel>
      <PanelHeader title={editing ? '编辑文章' : '新增文章'} description="手动录入、读取 TXT，或让 AI 按主题生成" action={editing && <Button variant="ghost" size="sm" onClick={cancelEdit}>取消编辑</Button>}/>
      <div className="panel-body stack">
        <div className="toolbar-row">
          <label className="button button-outline button-sm"><Upload size={14}/>读取 TXT<input hidden type="file" accept=".txt,text/plain" onChange={file}/></label>
        </div>
        <Field label="文章标题"><Input value={title} onChange={event => setTitle(event.target.value)} placeholder="输入文章标题"/></Field>
        <Field label="文章内容" hint={content ? `当前 ${charsOf(content)} 字（不含空白）` : undefined}><textarea className="textarea" rows={8} value={content} onChange={event => setContent(event.target.value)} placeholder="粘贴范文内容..."/></Field>
        <div><Button onClick={save} disabled={busy || !title.trim() || !content.trim()}>{editing ? <Pencil size={14}/> : <Plus size={14}/>} {busy ? '正在保存...' : editing ? '保存修改' : '添加文章'}</Button></div>
      </div>
    </Panel>
    <Modal open={!!preview} onOpenChange={value => !value && setPreview(null)} title={preview?.title || '文章预览'} description={preview ? `${charsOf(preview.content)} 字 · ${preview.mine ? '我' : preview.uploader || '未知教师'}` : undefined} footer={<Button variant="outline" onClick={() => setPreview(null)}>关闭</Button>}>
      <div className="article-preview">{preview?.content}</div>
    </Modal>
    <Modal open={!!confirm} onOpenChange={value => !value && setConfirm(null)} title="删除文章" description={`确定删除《${confirm?.title || ''}》吗？`} footer={<><Button variant="outline" onClick={() => setConfirm(null)}>取消</Button><Button variant="danger" onClick={remove}>删除</Button></>}><p style={{ fontSize: 13 }}>已被竞赛使用的文章不能删除。</p></Modal>
    <Modal open={aiOpen} onOpenChange={setAiOpen} title="AI 生成文章" description="按主题与字数生成范文，生成后直接写入文章库" footer={<><Button variant="outline" onClick={() => setAiOpen(false)}>取消</Button><Button onClick={generate} disabled={generating || !aiTopic.trim()}>{generating ? '正在生成，请稍候...' : '开始生成'}</Button></>}>
      <div className="stack">
        <Field label="文章主题" hint="例如：秋天的校园、拔河比赛、家乡的小河"><Input value={aiTopic} onChange={event => setAiTopic(event.target.value)} placeholder="输入一个主题" autoFocus/></Field>
        <Field label="体裁 / 风格"><AppSelect value={aiStyle} onValueChange={setAiStyle} items={STYLE_ITEMS}/></Field>
        <Field label="字数范围（不含空白）"><div style={{ display: 'flex', alignItems: 'center', gap: 8 }}><Input value={aiMin} onChange={event => setAiMin(event.target.value.replace(/[^0-9]/g, ''))} inputMode="numeric"/><span className="muted">—</span><Input value={aiMax} onChange={event => setAiMax(event.target.value.replace(/[^0-9]/g, ''))} inputMode="numeric"/></div></Field>
        <Field label="额外要求（可选）"><Input value={aiExtra} onChange={event => setAiExtra(event.target.value)} placeholder="例如：适合三年级、多用比喻句"/></Field>
        <div className="notice">生成通常需要 10–60 秒；模型与思考强度在「系统配置 → AI 文章生成」中设置。</div>
      </div>
    </Modal>
  </div>
}
