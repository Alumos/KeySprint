import { Download, Search, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { AppSelect, Button, Empty, Field, Input, Modal, Panel, PanelHeader, StatStrip } from '../../components/ui'
import { api, token } from '../../lib/api'
import type { ClassItem, Competition, Result, Student } from '../../lib/types'

type Mode='table'|'student'|'class'

export default function ResultsView({notify}:{notify:(message:string)=>void}) {
  const [results,setResults]=useState<Result[]>([])
  const [comps,setComps]=useState<Competition[]>([])
  const [classes,setClasses]=useState<ClassItem[]>([])
  const [competition,setCompetition]=useState(sessionStorage.getItem('resultCompetition')||'all')
  const [query,setQuery]=useState('')
  const [mode,setMode]=useState<Mode>('table')
  const [className,setClassName]=useState('')
  const [students,setStudents]=useState<Student[]>([])
  const [student,setStudent]=useState('')
  const [trend,setTrend]=useState<any[]>([])
  const [deleting,setDeleting]=useState<Result|null>(null)
  const [clearOpen,setClearOpen]=useState(false)

  async function load(){const[r,c,cl]=await Promise.all([api<Result[]>('/api/results'),api<Competition[]>('/api/competitions'),api<ClassItem[]>('/api/classes')]);setResults(r);setComps(c);setClasses(cl);sessionStorage.removeItem('resultCompetition')}
  useEffect(()=>{load()},[])
  useEffect(()=>{if(mode==='class'&&className)api<any[]>(`/api/analytics/class/${encodeURIComponent(className)}`).then(setTrend);if(mode==='student'&&className)api<Student[]>(`/api/analytics/class/${encodeURIComponent(className)}/students`).then(setStudents)},[className,mode])
  useEffect(()=>{if(mode==='student'&&student)api<any[]>(`/api/analytics/student/${encodeURIComponent(student)}`).then(setTrend)},[mode,student])

  const filtered=useMemo(()=>results.filter(x=>(competition==='all'||String(x.competition_id)===competition)&&(`${x.student_id} ${x.student_name} ${x.class_name||''} ${x.article_title}`).toLowerCase().includes(query.toLowerCase())),[competition,query,results])
  const avg=filtered.length?filtered.reduce((n,x)=>n+x.speed,0)/filtered.length:0
  const avgAcc=filtered.length?filtered.reduce((n,x)=>n+x.accuracy,0)/filtered.length*100:0
  const chartData=trend.map((x,i)=>({name:`${i+1}`,文章:x.article_title,speed:x.speed??x.avg_speed,accuracy:(x.accuracy!=null?x.accuracy*100:x.avg_accuracy)}))

  async function remove(){if(!deleting)return;try{await api(`/api/results/${deleting.id}`,{method:'DELETE'});setResults(items=>items.filter(item=>item.id!==deleting.id));setDeleting(null);notify('成绩已删除')}catch(e){notify((e as Error).message)}}
  async function clear(){try{await api('/api/results',{method:'DELETE'});setResults([]);setClearOpen(false);notify('全部成绩已清空')}catch(e){notify((e as Error).message)}}

  return <div className="admin-page">
    <StatStrip items={[{label:'成绩记录',value:filtered.length,detail:'当前筛选结果'},{label:'平均速度',value:avg.toFixed(1),detail:'字 / 分钟'},{label:'平均正确率',value:`${avgAcc.toFixed(1)}%`},{label:'最高速度',value:Math.max(0,...filtered.map(x=>x.speed)).toFixed(1),detail:filtered[0]?.student_name||'暂无成绩'}]}/>
    <Panel><PanelHeader title="分析视图" description="切换明细、个人追踪或班级平均趋势" action={<div className="segmented"><button className={mode==='table'?'active':''} onClick={()=>setMode('table')}>成绩明细</button><button className={mode==='student'?'active':''} onClick={()=>setMode('student')}>个人趋势</button><button className={mode==='class'?'active':''} onClick={()=>setMode('class')}>班级趋势</button></div>}/><div className="panel-body filters-panel">{mode==='table'?<><Field label="竞赛"><AppSelect value={competition} onValueChange={setCompetition} items={[{value:'all',label:'全部竞赛'},...comps.map(x=>({value:String(x.id),label:`${x.article_title} · ${x.access_code}`}))]}/></Field><Field label="搜索学生"><div className="input-icon"><Search size={14}/><Input value={query} onChange={e=>setQuery(e.target.value)} placeholder="学号、姓名或班级"/></div></Field><div className="spacer"/><Button variant="outline" onClick={()=>window.open(`/api/results/export/markdown?token=${token()}`)}><Download size={14}/>Markdown</Button><Button variant="outline" onClick={()=>window.open(`/api/results/export/csv?token=${token()}`)}><Download size={14}/>CSV</Button><Button variant="ghost" onClick={()=>setClearOpen(true)}><Trash2 size={14}/>清空</Button></>:<><Field label="班级"><AppSelect value={className} onValueChange={v=>{setClassName(v);setStudent('');setTrend([])}} placeholder="选择班级" items={classes.map(x=>({value:x.name,label:x.name}))}/></Field>{mode==='student'&&<Field label="学生"><AppSelect value={student} onValueChange={setStudent} disabled={!className} placeholder="选择学生" items={students.map(x=>({value:x.student_id||'',label:`${x.student_id} · ${x.student_name}`}))}/></Field>}</>}</div></Panel>
    {mode==='table'?<Panel><PanelHeader title="成绩明细" description={`共 ${filtered.length} 条记录；正确率 = 正确字数 ÷ 已输入字数，速度 = 每分钟正确字数`}/><div className="table-wrap">{filtered.length?<table className="data-table"><thead><tr><th>排名</th><th>学号</th><th>姓名</th><th>班级</th><th>文章</th><th>正确 / 已输入</th><th>正确率</th><th>速度</th><th>用时</th><th style={{textAlign:'right'}}>操作</th></tr></thead><tbody>{filtered.map((x,i)=><tr key={x.id}><td>{i+1}</td><td className="mono">{x.student_id}</td><td className="td-title">{x.student_name}</td><td>{x.class_name||'-'}</td><td className="td-title">{x.article_title}</td><td>{x.correct_count} / {x.typed_count}</td><td>{(x.accuracy*100).toFixed(1)}%</td><td><strong>{x.speed.toFixed(1)}</strong> 正确字/分</td><td>{x.duration} 秒</td><td><div className="row-actions"><Button size="icon" variant="ghost" onClick={()=>setDeleting(x)}><Trash2 size={14}/></Button></div></td></tr>)}</tbody></table>:<Empty title="没有匹配的成绩" description="调整上方筛选条件或开始一场竞赛"/>}</div></Panel>:<Panel><PanelHeader title={mode==='student'?'个人成绩趋势':'班级平均趋势'} description={trend.length?`共 ${trend.length} 场竞赛`:'请选择筛选条件'}/><div className="chart-wrap">{chartData.length?<ResponsiveContainer width="100%" height={340}><LineChart data={chartData} margin={{top:20,right:25,left:0,bottom:5}}><CartesianGrid stroke="#e4e4e7" strokeDasharray="3 3"/><XAxis dataKey="name" tick={{fontSize:11}}/><YAxis yAxisId="speed" tick={{fontSize:11}}/><YAxis yAxisId="accuracy" orientation="right" domain={[0,100]} tick={{fontSize:11}}/><Tooltip/><Legend/><Line yAxisId="speed" type="monotone" dataKey="speed" name="速度 (正确字/分)" stroke="#18181b" strokeWidth={2}/><Line yAxisId="accuracy" type="monotone" dataKey="accuracy" name="正确率 (%)" stroke="#2563eb" strokeWidth={2}/></LineChart></ResponsiveContainer>:<Empty title="暂无趋势数据" description="选择班级和学生后查看历史变化"/>}</div></Panel>}
    <Modal open={!!deleting} onOpenChange={v=>!v&&setDeleting(null)} title="删除成绩" description={`${deleting?.student_name||''} · ${deleting?.article_title||''}`} footer={<><Button variant="outline" onClick={()=>setDeleting(null)}>取消</Button><Button variant="danger" onClick={remove}>删除</Button></>}><p style={{fontSize:13}}>确定删除这条成绩记录吗？</p></Modal>
    <Modal open={clearOpen} onOpenChange={setClearOpen} title="清空全部成绩" description="此操作会删除当前教师的所有成绩" footer={<><Button variant="outline" onClick={()=>setClearOpen(false)}>取消</Button><Button variant="danger" onClick={clear}>全部清空</Button></>}><p style={{fontSize:13}}>竞赛与文章会保留，但成绩记录无法恢复。</p></Modal>
  </div>
}
